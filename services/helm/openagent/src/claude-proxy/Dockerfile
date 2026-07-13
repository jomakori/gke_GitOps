FROM oven/bun:latest
RUN apt-get update && apt-get install -y nodejs npm \
    && npm install -g @anthropic-ai/claude-code \
    && bun install -g claude-pipe \
    && rm -rf /var/lib/apt/lists/*
ENV CLAUDE_BIN=/usr/local/bin/claude
EXPOSE 4523
ENTRYPOINT ["claude-pipe"]
