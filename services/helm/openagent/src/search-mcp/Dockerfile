FROM node:22-alpine
RUN apk add --no-cache ripgrep
RUN npm install -g @atef_andrus/mcp-ripgrep
ENTRYPOINT ["mcp-ripgrep", "--allow-dir", "/data"]
