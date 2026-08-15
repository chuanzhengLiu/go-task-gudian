# ancient-texts-backend（古籍整理协作平台后端）

基于 Gin + GORM(MySQL) 的古籍整理协作平台后端服务，提供用户/机构管理、古籍项目管理、书页图像上传与切片、版本（异文）管理与导入、校对审核流程、JWT/TOTP 认证等 REST API。

## 标准命令

```bash
go build ./...          # 编译
go run ./cmd/server     # 启动（默认监听 :8080，需配置 .env，参考 .env.example）
go test ./...           # 测试（如有）
```

说明：

- 运行前复制 `.env.example` 为 `.env` 并按需修改（数据库 DSN、JWT 密钥、上传/切片目录等）。
- 服务启动需要可连接的 MySQL（`DB_DSN`），但编译和做题不依赖数据库。
- 容器内已预下载全部 Go 依赖，离线即可 `go build ./...`。
