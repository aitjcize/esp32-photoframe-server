/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_BASE_URL?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}

// JS-only package with no bundled type declarations.
declare module '@aitjcize/epaper-image-convert';
