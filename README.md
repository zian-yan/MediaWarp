[prs]: https://github.com/AkimioJR/MediaWarp
[prs-badge]: https://img.shields.io/badge/PRs-welcome-brightgreen.svg?style=flat-square
[issues]: https://github.com/AkimioJR/MediaWarp/issues/new
[issues-badge]: https://img.shields.io/badge/Issues-welcome-brightgreen.svg?style=flat-square
[release]: https://github.com/AkimioJR/MediaWarp/releases/latest
[release-badge]: https://img.shields.io/github/v/release/AkimioJR/MediaWarp?style=flat-square
[telegram]: https://t.me/AkimioJR_MediaWarp
[telegram-badge]: https://img.shields.io/badge/Telegram-blue?logo=Telegram


<div align="center">

# MediaWarp

MediaWarp 是**前置于 EmbyServer/Jellyfin 的反向代理服务器**，修改了原媒体服务器返回响应以实现 Strm 302 直链播放

[![prs][prs-badge]][prs]
[![issues][issues-badge]][issues]
[![release][release-badge]][release]
[![telegram][telegram-badge]][telegram]


[功能](#功能) •
[精简说明](#精简说明) •
[文档](#相关文档) •
[鸣谢](#鸣谢) •
[贡献者](#star-history) •
[Star History](#star-history)

</div>

# 功能

> 本分支为**精简版**，仅保留三项功能：**HTTPStrm 实现 302 重定向**、**屏蔽特定客户端访问**、**缓存与日志**。

- Strm 文件可以实现 302 直链播放，流量不经过 Emby Server / Jellyfin Server
  - **推荐配合 [AutoFilm](https://github.com/AkimioJR/AutoFilm) 使用**
  - 已通过测试客户端（Web、iOS Emby、Infuse、Conflux、Fileball、Vidhub）
  - 支持 Strm：HTTPStrm，Strm 文件内容是 HTTP 链接，浏览器访问链接可以直接下载到视频文件（**客户端需要可以访问到该链接，MediaWarp 不需要访问到该地址**）
  - 可选开启 `final_url`，重定向前先解析出最终地址，减少客户端重定向次数
  - 可选开启 `proxy`，允许保留媒体服务器的串流、转码能力

- 屏蔽特定客户端访问

  <img src="./img/client_filter.png" alt="" width=500px /> 

  - 按 User-Agent 关键字匹配，支持黑名单（BlackList）/ 白名单（WhiteList）两种模式

- 缓存与日志
  - 内存缓存：图片、字幕、HTTPStrm 重定向结果，TTL 可配置（`0` 表示关闭该项）
  - 日志：访问日志与服务日志分离，均支持输出到终端和按天切割的文件

- 配置方式：**只有配置文件一个入口**（`config/config.yaml`，参考 `config/config.yaml.example`）
  - 项目不包含任何 Web 页面或 Web 管理接口，编辑配置文件后**重启进程**生效
  - 启动时校验监听端口、上游地址、白名单名单等关键项，配置错误直接退出并打印原因

# 精简说明

以下功能已从本分支移除，如需要请使用[上游仓库](https://github.com/AkimioJR/MediaWarp)：

| 已移除 | 说明 |
| --- | --- |
| AlistStrm 302 重定向 | `alist_strm` 配置、Alist API 客户端、转码资源列表 |
| Web 前端美化 | `/MediaWarp/static`、`/MediaWarp/custom`、`robots.txt`、内置 JS 脚本、`custom` 目录 |
| Web 配置管理页面 | `/MediaWarp/config` 页面与 `/MediaWarp/api/config` 接口、`web` 配置段、配置热重载 |
| 字幕处理 | `subtitle` 配置、SRT 转 ASS、字体子集化 |
| 飞牛影视（FNTV）适配 | 媒体服务器类型仅剩 Emby、Jellyfin |
| 版本查询接口 | 已移除 `/MediaWarp/version`，保留 `--version` 命令行参数 |

# Docker 部署

镜像由 GitHub Actions 自动构建并推送到 GitHub Packages，镜像地址：`ghcr.io/zian-yan/mediawarp`

支持架构：`linux/amd64`、`linux/arm64`。`docker pull` 会自动拉取当前机器对应的架构，无需手动指定。

```bash
# 启动前先准备 /config/config.yaml（可参考 config/config.yaml.example）
docker run -d \
  --name mediawarp \
  --restart unless-stopped \
  -p 9000:9000 \
  -v /path/to/config:/config \
  -v /path/to/logs:/logs \
  -e TZ=Asia/Shanghai \
  ghcr.io/zian-yan/mediawarp:latest

# 也可以只拉取镜像
docker pull ghcr.io/zian-yan/mediawarp:latest
```

配置只通过 `config/config.yaml` 修改，改完重启容器生效（`docker restart mediawarp`）。容器内未内置 `config.yaml`，缺少该文件会启动失败；把 `config/config.yaml.example` 复制成 `config.yaml` 再改即可。

# 相关文档
- [教程文档](https://blog.akimio.top/posts/1041/)
- [更新日志](./docs/UpdateLog.md)
- [开发文档](./docs/DEV.md)
- [User-Agent参考](./docs/UA.md)

# 鸣谢
感谢一下人员、组织提供技术支持，仓库提供相关思路、脚本、前端样式。**排名不分先后**
- [chen3861229](https://github.com/chen3861229)
- [bpking1/embyExternalUrl](https://github.com/bpking1/embyExternalUrl)
- [newday-life/emby-front-end-mod](https://github.com/newday-life/emby-front-end-mod)
- [9channel/dd-danmaku](https://github.com/9channel/dd-danmaku)
- [Nolovenodie/emby-crx](https://github.com/Nolovenodie/emby-crx)
- [newday-life/jellyfin-crx](https://github.com/newday-life/jellyfin-crx)
- [RiderLty/fontInAss](https://github.com/RiderLty/fontInAss)

# 贡献者
<a href="https://github.com/AkimioJR/MediaWarp/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=AkimioJR/MediaWarp" />
</a>


# Star History
<a href="https://github.com/AkimioJR/MediaWarp/stargazers">
    <img width="500" alt="Star History Chart" src="https://api.star-history.com/svg?repos=AkimioJR/MediaWarp&type=Date">
</a> 