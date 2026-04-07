FROM node:20-alpine AS builder
WORKDIR /app
COPY frontend/package.json ./
RUN npm install
COPY frontend/ .
RUN npm run build

FROM nginx:1.25-alpine
# Remove default config
RUN rm /etc/nginx/conf.d/default.conf
# Copy our config
COPY nginx/nginx.conf /etc/nginx/conf.d/camguid.conf
# Copy built SPA
COPY --from=builder /app/dist /usr/share/nginx/html
EXPOSE 2005
CMD ["nginx", "-g", "daemon off;"]
