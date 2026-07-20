#!/usr/bin/env node

/**
 * epdoptimize-wrapper.mjs — CLI wrapper for the epdoptimize library.
 *
 * Usage:
 *   node epdoptimize-wrapper.mjs input.jpg output.png -d WxH -t thumb.jpg \
 *     [--palette-type spectra6|grayscale16|generic-2-color] \
 *     [--palette '<json>'] \
 *     [--settings '<json>'] \
 *     [--orientation landscape|portrait|portrait-upside-down|landscape-upside-down]
 *
 * The wrapper loads the input image, optionally scales/crops to target dimensions,
 * runs epdoptimize's dithering pipeline (auto or manual mode), and writes the
 * processed PNG and a JPEG thumbnail.
 */

import { readFile, writeFile } from "node:fs/promises";
import { createCanvas, loadImage } from "canvas";
import {
  ditherImage,
  replaceColors,
  suggestCanvasProcessingOptions,
  getProcessingPreset,
  spectra6Palette,
  aitjcizeSpectra6Palette,
  genericTwoColorEinkPalette,
  genericFourGrayscalePalette,
  trmnlSeeed16GrayscalePalette,
} from "epdoptimize";

// ── Argument parsing ──────────────────────────────────────────────────────────

function parseArgs(argv) {
  const args = argv.slice(2);
  const result = {
    input: null,
    output: null,
    dimension: null,
    thumb: null,
    paletteType: "spectra6",
    palette: null,
    settings: "{}",
    orientation: null,
  };

  for (let i = 0; i < args.length; i++) {
    const arg = args[i];
    switch (arg) {
      case "-d":
      case "--dimension":
        result.dimension = args[++i];
        break;
      case "-t":
      case "--thumb":
        result.thumb = args[++i];
        break;
      case "--palette-type":
        result.paletteType = args[++i];
        break;
      case "--palette":
        result.palette = args[++i];
        break;
      case "--settings":
        result.settings = args[++i];
        break;
      case "--orientation":
        result.orientation = args[++i];
        break;
      default:
        if (!result.input) {
          result.input = arg;
        } else if (!result.output) {
          result.output = arg;
        }
        break;
    }
  }

  if (!result.input || !result.output || !result.dimension) {
    console.error(
      "Usage: node epdoptimize-wrapper.mjs input.jpg output.png -d WxH -t thumb.jpg [options]"
    );
    process.exit(1);
  }

  return result;
}

// ── Palette resolution ────────────────────────────────────────────────────────

function resolvePalette(opts) {
  // If a raw palette JSON is provided (from epaper-image-convert format),
  // convert it to epdoptimize's PaletteColorEntry format
  if (opts.palette) {
    try {
      const parsed = JSON.parse(opts.palette);
      return convertPaletteFormat(parsed);
    } catch (e) {
      console.error("Warning: failed to parse palette JSON, falling back to default:", e.message);
    }
  }

  // Resolve by palette type
  switch (opts.paletteType) {
    case "grayscale16":
      return trmnlSeeed16GrayscalePalette;
    case "grayscale4":
      return genericFourGrayscalePalette;
    case "generic-2-color":
      return genericTwoColorEinkPalette;
    case "aitjcize-spectra6":
      return aitjcizeSpectra6Palette;
    case "spectra6":
    default:
      return spectra6Palette;
  }
}

/**
 * Convert epaper-image-convert palette format to epdoptimize PaletteColorEntry[].
 *
 * epaper-image-convert format:
 *   { "theoretical": { "black": {r,g,b}, ... },
 *     "perceived": { "black": {r,g,b}, "white": {r,g,b}, ... } }
 *
 * epdoptimize format:
 *   [ { name: "black", color: "#RRGGBB", deviceColor: "#RRGGBB" }, ... ]
 */
function convertPaletteFormat(wrapped) {
  const perceived = wrapped.perceived || {};
  const theoretical = wrapped.theoretical || {};

  const roleToDeviceColor = {
    black: "#000000",
    white: "#FFFFFF",
    yellow: "#FFFF00",
    red: "#FF0000",
    blue: "#0000FF",
    green: "#00FF00",
  };

  const entries = [];
  for (const [name, rgb] of Object.entries(perceived)) {
    if (!rgb || typeof rgb.r === "undefined") continue;
    const color = rgbToHex(rgb.r, rgb.g, rgb.b);
    const deviceColor = roleToDeviceColor[name] || color;
    entries.push({ name, color, deviceColor });
  }

  return entries.length > 0 ? entries : spectra6Palette;
}

function rgbToHex(r, g, b) {
  return (
    "#" +
    [r, g, b].map((v) => Math.max(0, Math.min(255, Math.round(v))).toString(16).padStart(2, "0")).join("")
  );
}

// ── Thumbnail generation ──────────────────────────────────────────────────────

function generateThumbnail(sourceCanvas, maxWidth = 200) {
  const scale = Math.min(1, maxWidth / sourceCanvas.width);
  const tw = Math.round(sourceCanvas.width * scale);
  const th = Math.round(sourceCanvas.height * scale);

  const thumbCanvas = createCanvas(tw, th);
  const ctx = thumbCanvas.getContext("2d");
  ctx.drawImage(sourceCanvas, 0, 0, tw, th);
  return thumbCanvas;
}

// ── Image loading and scaling ─────────────────────────────────────────────────

async function loadAndScale(inputPath, targetW, targetH, orientation) {
  const img = await loadImage(inputPath);

  let srcW = img.width;
  let srcH = img.height;

  // Target dimensions are always in native panel layout (landscape).
  // For portrait orientation, swap to process at portrait dimensions,
  // then rotate the output back to native landscape layout.
  let rotated = false;
  if (
    orientation &&
    (orientation.includes("portrait") || orientation === "portrait-upside-down")
  ) {
    // Swap target dimensions for portrait processing
    [targetW, targetH] = [targetH, targetW];
    rotated = true;
  }

  const canvas = createCanvas(targetW, targetH);
  const ctx = canvas.getContext("2d");

  // Cover-crop: fill the target area
  const srcAspect = srcW / srcH;
  const dstAspect = targetW / targetH;

  let sx, sy, sw, sh;
  if (srcAspect > dstAspect) {
    sh = srcH;
    sw = srcH * dstAspect;
    sx = (srcW - sw) / 2;
    sy = 0;
  } else {
    sw = srcW;
    sh = srcW / dstAspect;
    sx = 0;
    sy = (srcH - sh) / 2;
  }

  ctx.drawImage(img, sx, sy, sw, sh, 0, 0, targetW, targetH);

  // Rotate back to native panel layout if needed
  if (rotated) {
    const nativeW = img.width > img.height ? img.width : img.height;
    const nativeH = img.width > img.height ? img.height : img.width;
    // Re-read actual canvas dimensions (targetW/targetH are portrait now)
    const pw = targetW;
    const ph = targetH;
    const rotatedCanvas = createCanvas(ph, pw);
    const rctx = rotatedCanvas.getContext("2d");
    if (orientation === "portrait-upside-down") {
      rctx.translate(ph, pw);
      rctx.rotate(Math.PI);
    } else {
      rctx.translate(ph, 0);
      rctx.rotate(Math.PI / 2);
    }
    rctx.drawImage(canvas, 0, 0);
    return { canvas: rotatedCanvas, rotated: true };
  }

  return { canvas, rotated: false };
}

// ── Processing settings translation ───────────────────────────────────────────

function buildDitherOptions(settings, palette) {
  const opts = {
    palette,
    ditheringType: "errorDiffusion",
    errorDiffusionMatrix: "floydSteinberg",
    serpentine: true,
    colorMatching: "rgb",
  };

  if (!settings || settings.autoMode) {
    return opts; // Defaults; auto mode will override via suggestCanvasProcessingOptions
  }

  // If a preset is specified, use it as the base and override with any manual settings
  if (settings.preset) {
    const preset = getProcessingPreset(settings.preset);
    if (preset) {
      // Apply preset defaults
      if (preset.toneMapping) {
        opts.toneMapping = { ...preset.toneMapping };
      }
      if (preset.dynamicRangeCompression) {
        opts.dynamicRangeCompression = { ...preset.dynamicRangeCompression };
      }
      if (preset.colorMatching) {
        opts.colorMatching = preset.colorMatching;
      }
      if (preset.errorDiffusionMatrix) {
        opts.errorDiffusionMatrix = preset.errorDiffusionMatrix;
      }
    }
  }

  // Translate epaper-image-convert dither algorithm names to epdoptimize names
  const ditherMap = {
    "floyd-steinberg": "floydSteinberg",
    floydsteinberg: "floydSteinberg",
    floydSteinberg: "floydSteinberg",
    atkinson: "atkinson",
    "jarvis-judice-ninke": "jarvisJudiceNinke",
    jjn: "jarvisJudiceNinke",
    stucki: "stucki",
    burkes: "burkes",
  };

  if (settings.ditherAlgorithm) {
    const mapped = ditherMap[settings.ditherAlgorithm] || settings.ditherAlgorithm;
    opts.errorDiffusionMatrix = mapped;
  }

  // Color matching
  if (settings.colorMethod) {
    opts.colorMatching = settings.colorMethod;
  }

  // Tone mapping
  const toneMapping = {};
  let hasToneMapping = false;

  if (settings.toneMode) {
    toneMapping.mode = settings.toneMode;
    hasToneMapping = true;
  }

  // epaper-image-convert uses multiplier-based values (1.0 = neutral)
  // epdoptimize uses adjustment-based values (0 = neutral)
  if (settings.exposure !== undefined && settings.exposure !== "") {
    const multiplier = parseFloat(settings.exposure);
    if (!isNaN(multiplier) && multiplier !== 1.0) {
      toneMapping.exposure = Math.log2(multiplier);
      hasToneMapping = true;
    }
  }

  if (settings.saturation !== undefined && settings.saturation !== "") {
    const multiplier = parseFloat(settings.saturation);
    if (!isNaN(multiplier) && multiplier !== 1.0) {
      toneMapping.saturation = multiplier - 1;
      hasToneMapping = true;
    }
  }

  if (settings.contrast !== undefined && settings.contrast !== "") {
    const multiplier = parseFloat(settings.contrast);
    if (!isNaN(multiplier) && multiplier !== 1.0) {
      // epdoptimize contrast: 0 = neutral, positive = more contrast
      // epaper-image-convert: 1.0 = neutral, >1 = more contrast
      toneMapping.contrast = multiplier >= 1 ? multiplier - 1 : -(1 - multiplier) * 0.5;
      hasToneMapping = true;
    }
  }

  // S-curve parameters
  if (settings.toneMode === "scurve") {
    if (settings.scurveStrength !== undefined && settings.scurveStrength !== "") {
      toneMapping.strength = parseFloat(settings.scurveStrength);
      hasToneMapping = true;
    }
    if (settings.scurveShadow !== undefined && settings.scurveShadow !== "") {
      toneMapping.shadowBoost = parseFloat(settings.scurveShadow);
      hasToneMapping = true;
    }
    if (settings.scurveHighlight !== undefined && settings.scurveHighlight !== "") {
      toneMapping.highlightCompress = parseFloat(settings.scurveHighlight);
      hasToneMapping = true;
    }
    if (settings.scurveMidpoint !== undefined && settings.scurveMidpoint !== "") {
      toneMapping.midpoint = parseFloat(settings.scurveMidpoint);
      hasToneMapping = true;
    }
  }

  if (hasToneMapping) {
    opts.toneMapping = toneMapping;
  }

  // Dynamic range compression
  if (settings.compressDynamicRange === "" || settings.compressDynamicRange === true || settings.compressDynamicRange === "true") {
    opts.dynamicRangeCompression = { mode: "display", strength: 1 };
  }

  return opts;
}

// ── Main ──────────────────────────────────────────────────────────────────────

async function main() {
  const opts = parseArgs(process.argv);
  const settings = JSON.parse(opts.settings || "{}");

  // Parse target dimensions
  const [targetW, targetH] = opts.dimension.split("x").map(Number);
  if (!targetW || !targetH) {
    console.error("Invalid dimension format:", opts.dimension);
    process.exit(1);
  }

  // Load and scale image (handles orientation rotation back to native layout)
  const { canvas: sourceCanvas } = await loadAndScale(opts.input, targetW, targetH, opts.orientation);

  // Resolve palette
  const palette = resolvePalette(opts);

  // Build dither options
  const ditherOpts = buildDitherOptions(settings, palette);

  // Create output canvas
  const outputCanvas = createCanvas(targetW, targetH);

  if (settings.autoMode) {
    // Auto mode: use epdoptimize's image analysis to suggest optimal settings
    console.log("Auto mode: analyzing image for optimal processing settings...");
    const suggestion = suggestCanvasProcessingOptions(sourceCanvas, palette, {
      intent: "natural",
    });
    console.log("Suggestion:", JSON.stringify(suggestion.reasons, null, 2));

    // Merge suggested dither options with our base options
    const mergedOpts = {
      ...ditherOpts,
      ...suggestion.ditherOptions,
      palette, // Ensure palette is always present
    };

    await ditherImage(sourceCanvas, outputCanvas, mergedOpts);
  } else {
    // Manual mode: use the translated settings
    console.log("Manual mode: using provided settings");
    await ditherImage(sourceCanvas, outputCanvas, ditherOpts);
  }

  // Replace calibrated colors with native device colors
  const deviceCanvas = createCanvas(targetW, targetH);
  replaceColors(outputCanvas, deviceCanvas, palette);

  // Write output PNG
  const pngBuffer = deviceCanvas.toBuffer("image/png");
  await writeFile(opts.output, pngBuffer);
  console.log(`Wrote output: ${opts.output} (${pngBuffer.length} bytes)`);

  // Generate and write thumbnail
  if (opts.thumb) {
    const thumbCanvas = generateThumbnail(deviceCanvas, 200);
    const thumbBuffer = thumbCanvas.toBuffer("image/jpeg", { quality: 0.85 });
    await writeFile(opts.thumb, thumbBuffer);
    console.log(`Wrote thumbnail: ${opts.thumb} (${thumbBuffer.length} bytes)`);
  }
}

main().catch((err) => {
  console.error("Fatal error:", err);
  process.exit(1);
});
