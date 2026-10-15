/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** URL base del api-gateway. Es el único backend al que habla el frontend. */
  readonly VITE_API_URL?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
