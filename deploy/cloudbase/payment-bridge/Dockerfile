FROM node:24-alpine
WORKDIR /app
COPY package.json index.cjs index.js ./
ENV PORT=9000
EXPOSE 9000
USER node
CMD ["node", "index.js"]
