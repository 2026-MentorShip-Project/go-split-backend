#!/usr/bin/env node
import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import { createRequire } from 'node:module';
import { createClient } from './client.js';
import { registerTools } from './tools.js';

const DEFAULT_API_URL = 'https://go-backend-api-605450358080.asia-east1.run.app';

const token = process.env.GO_SPLIT_TOKEN;
if (!token) {
  console.error('GO_SPLIT_TOKEN is not set. Sign in to Go-Split and mint one with POST /auth/tokens.');
  process.exit(1);
}

const { version } = createRequire(import.meta.url)('../package.json');
const server = new McpServer({ name: 'go-split', version });
registerTools(server, createClient({ baseUrl: process.env.GO_SPLIT_API_URL ?? DEFAULT_API_URL, token }));
await server.connect(new StdioServerTransport());
