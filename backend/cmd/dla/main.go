package main

import (
    "bytes"
    "crypto/rand"
    "encoding/binary"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "math"
    "os"
    "path/filepath"
)

//
// ============================================================
// Configuration (match C++)
// ============================================================
//
const (
    W = 800
    H = 480

    // Growth (thin & clean)
    WALKERS_PER_FRAME = 84
    MAX_STEPS         = 6000

    // Post-process thickening (fractional avoids "+" artifacts)
    THICK_R = 1.1

)

var checkpointEvery = 0 // 0 = only final

//
// ============================================================
// Color + BMP
// ============================================================
//
type Color uint8

const (
    WHITE  Color = 0
    BLACK  Color = 1
    BLUE   Color = 2
    GREEN  Color = 3
    RED    Color = 4
    YELLOW Color = 5
)

type RGB struct{ r, g, b uint8 }

func toRGB(c Color) RGB {
    switch c {
    case BLACK:
        return RGB{0, 0, 0}
    case BLUE:
        return RGB{0, 0, 255}
    case GREEN:
        return RGB{0, 200, 0}
    case RED:
        return RGB{200, 0, 0}
    case YELLOW:
        return RGB{200, 200, 0}
    default:
        return RGB{255, 255, 255}
    }
}

func writeBMP24(path string, pix []Color) error {
    rowBytes := (3*W + 3) & ^3 // align to 4
    dataSize := rowBytes * H
    fileSize := 54 + dataSize

    f, err := os.Create(path)
    if err != nil {
        return fmt.Errorf("cannot write BMP %s: %w", path, err)
    }
    defer f.Close()

    // BMP header (54 bytes), little-endian fields
    header := make([]byte, 54)
    header[0] = 'B'
    header[1] = 'M'
    binary.LittleEndian.PutUint32(header[2:], uint32(fileSize))
    // [6:10] reserved = 0
    binary.LittleEndian.PutUint32(header[10:], 54) // pixel data offset
    binary.LittleEndian.PutUint32(header[14:], 40) // DIB header size
    binary.LittleEndian.PutUint32(header[18:], uint32(W))
    binary.LittleEndian.PutUint32(header[22:], uint32(H))
    binary.LittleEndian.PutUint16(header[26:], 1)  // planes
    binary.LittleEndian.PutUint16(header[28:], 24) // bpp
    // compression=0
    binary.LittleEndian.PutUint32(header[34:], uint32(dataSize))

    if _, err := f.Write(header); err != nil {
        return err
    }

    row := make([]byte, rowBytes)

    // Bottom-up rows
    for y := H - 1; y >= 0; y-- {
        i := 0
        base := y * W
        for x := 0; x < W; x++ {
            c := toRGB(pix[base+x])
            row[i+0] = c.b
            row[i+1] = c.g
            row[i+2] = c.r
            i += 3
        }
        for k := i; k < len(row); k++ {
            row[k] = 0
        }
        if _, err := f.Write(row); err != nil {
            return err
        }
    }
    return nil
}

//
// ============================================================
// Helpers
// ============================================================
//
func wrap(x, m int) int {
    if x < 0 {
        return x + m
    }
    if x >= m {
        return x - m
    }
    return x
}

func clamp(v, lo, hi int) int {
    if v < lo {
        return lo
    }
    if v > hi {
        return hi
    }
    return v
}

//
// ============================================================
// Small, fast RNG (xoshiro256** + splitmix64 seeding)
// ============================================================
//
func rotl64(x uint64, k int) uint64 {
    return (x << k) | (x >> (64 - k))
}

type SplitMix64 struct{ x uint64 }

func (s *SplitMix64) Seed(seed uint64) { s.x = seed }

func (s *SplitMix64) Next() uint64 {
    s.x += 0x9E3779B97F4A7C15
    z := s.x
    z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
    z = (z ^ (z >> 27)) * 0x94D049BB133111EB
    return z ^ (z >> 31)
}

type Xoshiro256ss struct{ s [4]uint64 }

func (x *Xoshiro256ss) Seed(seed uint64) {
    var sm SplitMix64
    sm.Seed(seed)
    x.s[0] = sm.Next()
    x.s[1] = sm.Next()
    x.s[2] = sm.Next()
    x.s[3] = sm.Next()
}

func (x *Xoshiro256ss) NextU64() uint64 {
    result := rotl64(x.s[1]*5, 7) * 9
    t := x.s[1] << 17

    x.s[2] ^= x.s[0]
    x.s[3] ^= x.s[1]
    x.s[1] ^= x.s[2]
    x.s[0] ^= x.s[3]
    x.s[2] ^= t
    x.s[3] = rotl64(x.s[3], 45)

    return result
}

func (x *Xoshiro256ss) NextU32() uint32 { return uint32(x.NextU64() >> 32) }

// Unbiased bounded int using rejection sampling (match C++)
func randInt(rng *Xoshiro256ss, lo, hi int) int {
    rngRange := uint32(hi - lo + 1)
    limit := uint32(0xFFFFFFFF) - (uint32(0xFFFFFFFF) % rngRange)
    var v uint32
    for {
        v = rng.NextU32()
        if v < limit {
            break
        }
    }
    return lo + int(v%rngRange)
}

//
// ============================================================
// Bit-packed occupancy grid
// ============================================================
//
type BitGrid struct {
    bits []uint64
}

func NewBitGrid() BitGrid {
    n := (W*H + 63) / 64
    return BitGrid{bits: make([]uint64, n)}
}

func (g *BitGrid) Clear() {
    for i := range g.bits {
        g.bits[i] = 0
    }
}

func (g *BitGrid) GetXY(x, y int) bool {
    i := y*W + x
    return ((g.bits[i>>6] >> uint(i&63)) & 1) != 0
}

func (g *BitGrid) SetXY(x, y int) {
    i := y*W + x
    g.bits[i>>6] |= 1 << uint(i&63)
}

//
// ============================================================
// Layer
// ============================================================
//
type Layer struct {
    occ   BitGrid
    color Color
    rng   Xoshiro256ss
}

//
// ============================================================
// Checkpoint I/O (binary occupancy + binary RNG)
// ============================================================
//
type checkpointJSON struct {
    Frame int `json:"frame"`
}

func saveCheckpoint(dir string, frame int, layers []Layer) error {
    jsonTmp := filepath.Join(dir, "checkpoint.json.tmp")
    binTmp := filepath.Join(dir, "checkpoint.bin.tmp")
    jsonOut := filepath.Join(dir, "checkpoint.json")
    binOut := filepath.Join(dir, "checkpoint.bin")

    // JSON
    {
        f, err := os.Create(jsonTmp)
        if err != nil {
            return err
        }
        enc := json.NewEncoder(f)
        enc.SetIndent("", " ")
        if err := enc.Encode(checkpointJSON{Frame: frame}); err != nil {
            f.Close()
            return err
        }
        if err := f.Close(); err != nil {
            return err
        }
    }

    // BIN (little-endian; keep "DLA1" header)
    {
        f, err := os.Create(binTmp)
        if err != nil {
            return err
        }
        defer f.Close()

        const magic uint32 = 0x444C4131 // "DLA1"
        if err := binary.Write(f, binary.LittleEndian, magic); err != nil {
            return err
        }
        if err := binary.Write(f, binary.LittleEndian, uint32(W)); err != nil {
            return err
        }
        if err := binary.Write(f, binary.LittleEndian, uint32(H)); err != nil {
            return err
        }
        if err := binary.Write(f, binary.LittleEndian, uint32(len(layers))); err != nil {
            return err
        }

        for i := range layers {
            L := &layers[i]
            nWords := uint32(len(L.occ.bits))
            if err := binary.Write(f, binary.LittleEndian, nWords); err != nil {
                return err
            }
            if err := binary.Write(f, binary.LittleEndian, L.occ.bits); err != nil {
                return err
            }
            if err := binary.Write(f, binary.LittleEndian, L.rng.s); err != nil {
                return err
            }
        }

        if err := f.Sync(); err != nil {
            return err
        }
    }

    // Atomic-ish rename (same filesystem)
    if err := os.Rename(jsonTmp, jsonOut); err != nil {
        _ = os.Remove(jsonTmp)
        return err
    }
    if err := os.Rename(binTmp, binOut); err != nil {
        _ = os.Remove(binTmp)
        return err
    }
    return nil
}

func loadCheckpoint(dir string, layers []Layer) (frame int, err error) {
    jsonPath := filepath.Join(dir, "checkpoint.json")
    binPath := filepath.Join(dir, "checkpoint.bin")

    // JSON
    {
        b, e := os.ReadFile(jsonPath)
        if e != nil {
            return 0, e
        }
        var cj checkpointJSON
        if e := json.Unmarshal(b, &cj); e != nil {
            return 0, e
        }
        frame = cj.Frame
    }

    // BIN
    f, err := os.Open(binPath)
    if err != nil {
        return 0, err
    }
    defer f.Close()

    var magic, w, h, nLayers uint32
    if err := binary.Read(f, binary.LittleEndian, &magic); err != nil {
        return 0, err
    }
    if err := binary.Read(f, binary.LittleEndian, &w); err != nil {
        return 0, err
    }
    if err := binary.Read(f, binary.LittleEndian, &h); err != nil {
        return 0, err
    }
    if err := binary.Read(f, binary.LittleEndian, &nLayers); err != nil {
        return 0, err
    }

    if magic != 0x444C4131 || int(w) != W || int(h) != H || int(nLayers) != len(layers) {
        return 0, errors.New("invalid checkpoint header")
    }

    for i := range layers {
        L := &layers[i]
        var nWords uint32
        if err := binary.Read(f, binary.LittleEndian, &nWords); err != nil {
            return 0, err
        }
        if int(nWords) != len(L.occ.bits) {
            return 0, errors.New("invalid checkpoint word count")
        }
        if err := binary.Read(f, binary.LittleEndian, L.occ.bits); err != nil {
            return 0, err
        }
        if err := binary.Read(f, binary.LittleEndian, &L.rng.s); err != nil {
            return 0, err
        }
    }

    // Extra trailing bytes are harmless; ignore
    _, _ = io.Copy(io.Discard, f)

    return frame, nil
}

//
// ============================================================
// Thickening (fractional Euclidean radius on periodic domain)
// ============================================================
//
func thicken(src *BitGrid, dst *[]uint8) {
    out := make([]uint8, W*H)

    // 2× supersample
    SW := 2 * W
    SH := 2 * H

    hiOcc := make([]uint8, SW*SH)
    hiDil := make([]uint8, SW*SH)

    // Map each low-res occupied pixel to center of 2×2 block (odd coords)
    for y := 0; y < H; y++ {
        for x := 0; x < W; x++ {
            if !src.GetXY(x, y) {
                continue
            }
            hx := 2*x + 1
            hy := 2*y + 1
            hiOcc[hy*SW+hx] = 1
        }
    }

    Rhi := THICK_R * 2.0
    R := int(math.Ceil(Rhi))
    R2 := Rhi * Rhi

    wrapHi := func(v, m int) int {
        if v < 0 {
            return v + m
        }
        if v >= m {
            return v - m
        }
        return v
    }

    // Stamp disk around each occupied high-res point
    for hy := 0; hy < SH; hy++ {
        rowBase := hy * SW
        for hx := 0; hx < SW; hx++ {
            if hiOcc[rowBase+hx] == 0 {
                continue
            }
            for dy := -R; dy <= R; dy++ {
                for dx := -R; dx <= R; dx++ {
                    if float64(dx*dx+dy*dy) > R2 {
                        continue
                    }
                    xx := wrapHi(hx+dx, SW)
                    yy := wrapHi(hy+dy, SH)
                    hiDil[yy*SW+xx] = 1
                }
            }
        }
    }

    // Downsample: if any pixel in 2×2 block is set
    for y := 0; y < H; y++ {
        by := 2 * y
        for x := 0; x < W; x++ {
            bx := 2 * x
            v := hiDil[(by+0)*SW+(bx+0)] |
                hiDil[(by+0)*SW+(bx+1)] |
                hiDil[(by+1)*SW+(bx+0)] |
                hiDil[(by+1)*SW+(bx+1)]
            if v != 0 {
                out[y*W+x] = 1
            }
        }
    }

    *dst = out
}

//
// ============================================================
// One frame of growth for one layer (optimized inner loop)
// ============================================================
//
var dirs = [8][2]int8{
    {-1, -1}, {0, -1}, {1, -1},
    {-1, 0}, {1, 0},
    {-1, 1}, {0, 1}, {1, 1},
}

func advanceOneFrame(L *Layer) {
    for p := 0; p < WALKERS_PER_FRAME; p++ {
        x := randInt(&L.rng, 0, W-1)
        y := randInt(&L.rng, 0, H-1)
        for s := 0; s < MAX_STEPS; s++ {
            si := randInt(&L.rng, 0, 7)
            dx := int(dirs[si][0])
            dy := int(dirs[si][1])

            x += dx
            if x < 0 {
                x += W
            } else if x >= W {
                x -= W
            }
            y += dy
            if y < 0 {
                y += H
            } else if y >= H {
                y -= H
            }

            x0 := x
            y0 := y
            xl := x0 - 1
            if xl < 0 {
                xl = W - 1
            }
            xr := x0 + 1
            if xr >= W {
                xr = 0
            }
            yu := y0 - 1
            if yu < 0 {
                yu = H - 1
            }
            yd := y0 + 1
            if yd >= H {
                yd = 0
            }

            if L.occ.GetXY(xl, yu) ||
                L.occ.GetXY(x0, yu) ||
                L.occ.GetXY(xr, yu) ||
                L.occ.GetXY(xl, y0) ||
                L.occ.GetXY(xr, y0) ||
                L.occ.GetXY(xl, yd) ||
                L.occ.GetXY(x0, yd) ||
                L.occ.GetXY(xr, yd) {
                L.occ.SetXY(x0, y0)
                break
            }
        }
    }
}

//
// ============================================================
// Main
// ============================================================
//
func usage() {
    fmt.Fprintln(os.Stderr, "Usage:")
    fmt.Fprintln(os.Stderr, "  dla_fast out --init")
    fmt.Fprintln(os.Stderr, "  dla_fast out --to N")
}

func randSeedU64() (uint64, error) {
    var b [8]byte
    if _, err := rand.Read(b[:]); err != nil {
        return 0, err
    }
    return binary.LittleEndian.Uint64(b[:]), nil
}

func main() {
    if len(os.Args) < 3 {
        usage()
        os.Exit(1)
    }

    outDir := os.Args[1]
    mode := os.Args[2]

    if err := os.MkdirAll(outDir, 0o755); err != nil {
        fmt.Fprintln(os.Stderr, "ERROR:", err)
        os.Exit(1)
    }

    doInit := mode == "--init"
    doTo := mode == "--to"

    targetFrame := 0
    if doTo {
        if len(os.Args) < 4 {
            fmt.Fprintln(os.Stderr, "Missing N for --to")
            os.Exit(1)
        }
        _, err := fmt.Sscanf(os.Args[3], "%d", &targetFrame)
        if err != nil {
            fmt.Fprintln(os.Stderr, "Invalid N for --to:", err)
            os.Exit(1)
        }
        if targetFrame < 0 {
            targetFrame = 0
        }
    } else if !doInit {
        fmt.Fprintln(os.Stderr, "Unknown mode. Use --init or --to N")
        os.Exit(1)
    }

    // Allocate layers
    layers := make([]Layer, 5)
    for i := range layers {
        layers[i].occ = NewBitGrid()
        layers[i].color = WHITE
    }

    pal := [5]Color{BLUE, GREEN, RED, YELLOW, BLACK}

    // ---------- INIT ----------
    if doInit {
        seed, err := randSeedU64()
        if err != nil {
            fmt.Fprintln(os.Stderr, "ERROR: cannot seed RNG:", err)
            os.Exit(1)
        }

        var sm SplitMix64
        sm.Seed(seed)
        urand01 := func() float64 {
            // (next >> 11) / 2^53
            return float64(sm.Next()>>11) * (1.0 / 9007199254740992.0)
        }

        cols, rows := 3, 2
        k := 0
        for r := 0; r < rows && k < 5; r++ {
            for c := 0; c < cols && k < 5; c++ {
                jx := (urand01() - 0.5) * 0.5 // [-0.25, 0.25]
                jy := (urand01() - 0.5) * 0.5

                x := int((float64(c)+0.5+jx)*float64(W) / float64(cols))
                y := int((float64(r)+0.5+jy)*float64(H) / float64(rows))
                x = clamp(x, 0, W-1)
                y = clamp(y, 0, H-1)

                layers[k].color = pal[k]
                layers[k].occ.Clear()

                // seed tuft 3x3
                for dy := -1; dy <= 1; dy++ {
                    for dx := -1; dx <= 1; dx++ {
                        xx := wrap(x+dx, W)
                        yy := wrap(y+dy, H)
                        layers[k].occ.SetXY(xx, yy)
                    }
                }

                layers[k].rng.Seed(seed + uint64(k)*0x9E3779B97F4A7C15)
                k++
            }
        }

        if err := saveCheckpoint(outDir, 0, layers); err != nil {
            fmt.Fprintln(os.Stderr, "ERROR: failed to write checkpoint:", err)
            os.Exit(1)
        }
        fmt.Println("Initialized frame 0")
        return
    }

    // ---------- RESUME ----------
    curFrame, err := loadCheckpoint(outDir, layers)
    if err != nil {
        fmt.Fprintln(os.Stderr, "No valid checkpoint found. Run --init first.")
        fmt.Fprintln(os.Stderr, "Details:", err)
        os.Exit(1)
    }
    for i := 0; i < 5; i++ {
        layers[i].color = pal[i]
    }
    fmt.Println("Resuming from frame", curFrame)

    // ---------- ADVANCE ----------
    if curFrame < targetFrame {
        for f := curFrame + 1; f <= targetFrame; f++ {
            for i := range layers {
                advanceOneFrame(&layers[i])
            }
            if checkpointEvery > 0 && (f%checkpointEvery == 0) {
                if err := saveCheckpoint(outDir, f, layers); err != nil {
                    fmt.Fprintln(os.Stderr, "ERROR: failed checkpoint at frame", f, ":", err)
                    os.Exit(1)
                }
            }
        }
        if err := saveCheckpoint(outDir, targetFrame, layers); err != nil {
            fmt.Fprintln(os.Stderr, "ERROR: failed to write final checkpoint:", err)
            os.Exit(1)
        }
    } else {
        fmt.Println("Already at or beyond requested frame", targetFrame, "(render only)")
    }

    // ---------- RENDER ----------
    img := make([]Color, W*H)
    for i := range img {
        img[i] = WHITE
    }
    var thick []uint8

    for i := 0; i < 5; i++ {
        thicken(&layers[i].occ, &thick)
        for p := 0; p < len(img); p++ {
            if thick[p] != 0 {
                img[p] = layers[i].color
            }
        }
    }

    outPath := filepath.Join(outDir, "latest_display.bmp")
    if err := writeBMP24(outPath, img); err != nil {
        fmt.Fprintln(os.Stderr, "ERROR:", err)
        os.Exit(1)
    }
    fmt.Println("Done. Frame", targetFrame, "saved.")
}

// Small helper for fmt.Sscanf error messages without importing strconv just for int parsing.
func init() {
    // Ensure fmt isn't optimized away (not needed, but keeps static analyzers calm).
    _ = bytes.MinRead
}
