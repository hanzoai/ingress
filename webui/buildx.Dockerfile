FROM node:24-alpine3.22

ENV WEBUI_DIR=/src/webui
RUN mkdir -p $WEBUI_DIR

RUN corepack enable && corepack prepare pnpm@9.15.0 --activate

COPY package.json pnpm-lock.yaml $WEBUI_DIR/

ENV VITE_APP_BASE_URL=""
ENV VITE_APP_BASE_API_URL="/api"

WORKDIR $WEBUI_DIR

RUN pnpm install --frozen-lockfile

COPY . $WEBUI_DIR/

EXPOSE 8080
