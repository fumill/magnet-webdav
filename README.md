# Magnet WebDAV Server

基于 Go 语言的磁力链接 WebDAV 服务器，支持在线播放和 CDN 加速。

## 功能特性

- 🚀 **高性能**: 基于 Go 语言开发，支持高并发
- 🔗 **磁力解析**: 自动解析磁力链接并获取文件列表
- 📺 **在线播放**: 支持视频流式播放，无需完整下载
- 🌐 **WebDAV 支持**: 兼容 Kodi、Infuse 等客户端
- 💾 **多数据库**: 支持 SQLite 和 PostgreSQL
- 🐳 **Docker 部署**: 支持容器化部署

### 快速开始

1. 创建配置文件：
```bash
mkdir -p data/db data/torrents
cp config.example.yaml config.yaml
# 编辑 config.yaml 调整配置（可选）
```
2.自定义配置
```yaml
version: '3.8'

services:
  magnet-webdav:
    environment:
      - ENV=production
      - DB_DRIVER=postgres
      - DB_HOST=postgres
      - DB_USER=postgres
      - DB_PASSWORD=yourpassword
      - DB_NAME=magnet_webdav
      - TORRENT_DIR=/data/torrents
    volumes:
      - ./config.yaml:/app/config.yaml
    command: ["./main", "-c", "/app/config.yaml"]

```

## 数据持久化
所有数据都保存在 ./data 目录中：

./data/db - 数据库文件

./data/torrents - 种子缓存文件

## 健康检查
```
curl http://localhost:3000/health
```

## 环境变量
| 环境变量 | 说明 | 默认值 |
|---------|------|--------|
| PORT | 服务端口 | 3000 |
| ENV | 运行环境 | development |
| DB_DRIVER | 数据库驱动 | sqlite |
| DB_NAME | 数据库名称 | magnet_webdav.db |
| TORRENT_DIR | 种子下载目录 | /data/torrents |
| AUTH_ENABLED | 启用 WebDAV 认证 | false |
| WEBDAV_USERNAME | WebDAV 用户名 | admin |
| WEBDAV_PASSWORD | WebDAV 密码 | password |
