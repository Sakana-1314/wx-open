/// <reference types="vite/client" />

/**
 * 构建期由 Vite `define` 注入的后端 HOST（环境变量 HOST）。
 * 值为空、裸域名（如 https://example.com）或已含路径（如 https://example.com/api/v1）均可，
 * 具体解析见 src/api/config.ts。
 */
declare const __API_HOST__: string | undefined