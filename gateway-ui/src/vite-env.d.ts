/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** 可选：直连后端地址（默认走同源 /api 由 Vite 代理转发） */
  readonly VITE_API_BASE?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
