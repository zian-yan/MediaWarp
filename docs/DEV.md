# 开发相关文档

## 项目目录结构
```
MEDIAWARP
├─.github
│  └─workflows          # 工作流相关
├─config                # MediaWarp配置文件
├─constants             # 存放一些常量（媒体服务器类型、正则、Strm 类型）
├─docs                  # MediaWarp相关文档
├─docker                # Dockerfile（源码构建）与 Dockerfile.ci（预编译产物打包）
├─img                   # MediaWarp演示图片
├─internal
│  ├─config             # 配置读取、启动期校验与目录管理
│  ├─handler            # 请求处理：HTTPStrm 302、PlaybackInfo 修正、反向代理
│  ├─logging            # 访问日志 / 服务日志
│  ├─middleware         # 访问日志、panic 恢复、客户端过滤、图片/字幕缓存
│  ├─router             # 路由与中间件链
│  └─service            # Emby / Jellyfin 开放接口客户端
├─logs                  # MediaWarp输出日志
└─utils                 # 一些工具集合
```

## 配置加载

配置只有配置文件一个入口，**不存在 Web 页面或 Web 管理接口**，改完需重启进程。

- 入口：`config.Init(path)`，`--config` 可指定路径，默认 `config/config.yaml`
- 流程：`os.ReadFile` → `yaml.Unmarshal` 到 `Setting` → 赋值到包级变量 → `validate()`
- 加载只发生一次：启动后没有任何回写、热重载或配置监听逻辑
- 失败处理：任一步返回错误，`main` 直接 panic 并打印原因（退出码非 0）

启动期校验（`validate`）只拦「配错一定跑不起来」的项：

| 校验项 | 规则 |
| --- | --- |
| `port` | 1-65535，缺失或为 0 直接报错；超过 uint16 在 yaml 解析阶段就会失败 |
| `server.type` | 只能 `Emby` / `Jellyfin`，其他值在 yaml 解析阶段失败 |
| `server.addr` | 非空、必须 `http://` 或 `https://` 开头、必须有主机名 |
| `client.list` | 白名单模式下不能为空，否则所有请求都会被 403 |

以下属于合法配置，不做拦截：缓存 TTL ≤ 0（关闭该项缓存）、`http_strm.enable` 为 true 但 `prefix_list` 为空（启动时打印警告，302 不会命中）。

配置文件字段（`internal/config/type.go`）：

| 段 | 字段 | 说明 |
| --- | --- | --- |
| 顶层 | `port` | MediaWarp 监听端口 |
| `server` | `type` / `addr` / `auth` | 上游媒体服务器类型、地址（端口即代理端口）、API 密钥 |
| `log` | `access.console/file`、`service.console/file` | 两套日志各自是否输出到终端 / 文件 |
| `cache` | `enable`、`http_strm_ttl`、`image_ttl`、`subtitle_ttl` | 缓存总开关与各项 TTL（时长格式 `1m` / `2h`，≤ 0 关闭） |
| `client` | `enable`、`mode`、`list` | 客户端过滤开关、黑白名单模式与 UA 关键字 |
| `http_strm` | `enable`、`proxy`、`final_url`、`compatibility_mode`、`prefix_list` | 302 重定向相关开关与 Strm 路径前缀 |

> 旧配置文件里残留的 `web:` 段会被 yaml 静默忽略，不影响启动，但已无任何作用。

## 启动流程

1. `flag.Parse()` 解析 `--version` / `--debug` / `--config`
2. `config.Init` 加载并校验配置
3. `logging.Init` 按配置挂载终端输出与按天切割的文件钩子
4. `handler.Init` 按 `server.type` 构建上游处理器（Emby / Jellyfin），全局唯一，运行期不再变更
5. `router.InitRouter` 组装中间件链与正则路由，缓存池在此一次性建立
6. `net.Listen` 显式监听 `:port`，端口不可用立刻 panic；随后 `http.Server.Serve`
7. 收到 `SIGINT` / `SIGTERM` 后 5 秒优雅退出

## Docker 镜像与发布

三个工作流分工：`build-binary.yaml` 交叉编译 6 个平台二进制（Linux amd64/arm64、macOS amd64/arm64、Windows amd64/arm64）→ `build-docker.yaml` 把其中 Linux 的两个打进镜像并推送 → `dev.yaml`（push main）/`release.yaml`（push tag）负责编排。

`build-docker.yaml` 为 `workflow_call`，输入参数：

| 输入 | 说明 |
| --- | --- |
| `push` | 是否推送镜像；`false` 仅构建 |
| `tags` | `docker/metadata-action` 的 tag 规则，如 `type=ref,event=tag` |
| `platforms` | 目标平台，默认 `linux/amd64,linux/arm64` |

发布目标（自动判断）：

- `ghcr.io/<owner>/<repo>`：始终发布，使用内置 `GITHUB_TOKEN`，需要 `packages: write` 权限
- `docker.io/<DOCKERHUB_USERNAME>/mediawarp`：仅当 `DOCKERHUB_USERNAME` 与 `DOCKERHUB_TOKEN` 都已配置时才发布，未配置会在日志里给出 notice 并跳过
- 镜像名统一转小写（GitHub 仓库名含大写时 ghcr 会拒绝推送）

产物映射：`docker/Dockerfile.ci` 用 `COPY --chmod=755 targets/${TARGETPLATFORM}/MediaWarp`，所以工作流把 artifact 按 `targets/linux/amd64/MediaWarp`、`targets/linux/arm64/MediaWarp` 摆放。目前只支持 `linux/amd64` 与 `linux/arm64` 两个平台，平台与 artifact 的对应关系写在同一处的 `case` 分支里；新增平台需同时改 `build-binary.yaml` 矩阵与这个 `case`，否则会在准备阶段显式报错。

`docker/Dockerfile` 是源码构建版（多阶段，`golang` 构建 + `alpine` 运行），供本地构建与 CI 校验使用；CI 的 `docker-build` 作业只构建不推送。

## Release 正文（changelog）的版本区间

`release.yaml` 的 `Generate Release Body` 只调用 `scripts/changelog.sh <tag>`，区间界定规则按优先级：

1. 显式基线：仓库变量 `CHANGELOG_BASE`（或脚本第 2 个参数），区间为 `BASE..TAG`
2. 自动：`git describe --tags --abbrev=0 --match 'v[0-9]*.[0-9]*.[0-9]*' 'TAG^'`，即当前 tag 之前最近的版本 tag
3. 兜底：所有版本 tag 中「是当前 tag 祖先、且提交时间最近」的那个
4. 都没有：判定为首个版本，**只写说明、不列任何提交**（不再回退成 `git log HEAD` 全量历史）

注意第 2 条不能用 `git tag --sort=-v:refname` 取版本号最大的 tag：维护分支上的 tag 版本号可能更大但与本次发布线无关，会把已发布内容重新算进本版本。脚本因此按提交拓扑取基线，并把「首个版本」与「区间为空」分别输出明确文案。

脚本依赖完整历史与 tag，`actions/checkout` 必须保持 `fetch-depth: 0`；tag 解析失败时脚本以退出码 2 直接失败，避免发出区间错误的正文。

本地验证：

```bash
bash scripts/changelog.sh v1.0.0     # 预览某个 tag 的 release 正文
bash scripts/changelog_selftest.sh   # 区间界定自测（在临时仓库里造历史，不动当前仓库）
```

首个版本（仓库里没有更早的版本 tag）生成的正文只有一句说明和提交列表链接。若首版内容较多，可人工补一段「### 本版本要点」——要点从 `docs/UpdateLog.md` 对应版本条目摘取，放在「完整更新记录」之前，然后手动更新该 release。这只针对首版，之后有基线就只由脚本产出，不再人工改动正文。

# 本地构建

## 编译二进制
```bash
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o MediaWarp .
```

## 构建镜像
```bash
# 单架构（本机）
docker build -f docker/Dockerfile -t mediawarp:dev .

# 多架构（需要 buildx + QEMU）
docker buildx build --platform linux/amd64,linux/arm64 \
  -f docker/Dockerfile -t mediawarp:dev --load .
```

# 媒体服务器部分 API 具体响应
## EmbyServer
### playbackInfo
**EmbyServer version 4.8**
AlistStrm

```
{
    "MediaSources": [
        {
            "Protocol": "File",
            "Id": "37bcb3827aeb2095f1b6293e9a189072",
            "Path": "/资源盘/ANIOpen/2024-10/[ANi] 魔王陛下，RETRY！R - 04 [1080P][Baha][WEB-DL][AAC AVC][CHT].mp4",
            "Type": "Default",
            "Container": "strm",
            "Size": 0,
            "Name": "重来吧，魔王大人！ S02E04  -1080p -AVC -AAC -ANi -Baha",
            "IsRemote": false,
            "HasMixedProtocols": false,
            "SupportsTranscoding": true,
            "SupportsDirectStream": false,
            "SupportsDirectPlay": false,
            "IsInfiniteStream": false,
            "RequiresOpening": false,
            "RequiresClosing": false,
            "RequiresLooping": false,
            "SupportsProbing": false,
            "MediaStreams": [],
            "Formats": [],
            "RequiredHttpHeaders": {},
            "DirectStreamUrl": "/videos/61162/master.m3u8?DeviceId=e12ab84a-2948-463a-9cb0-ecebcac2949e&MediaSourceId=37bcb3827aeb2095f1b6293e9a189072&PlaySessionId=e643532a23ca4a60b2c70d4210c018a9&api_key=e824c422e02047dfa820dbcab5490bb9&VideoCodec=h264,h265,hevc,av1&VideoBitrate=1400000&TranscodingMaxAudioChannels=2&SegmentContainer=ts&MinSegments=1&BreakOnNonKeyFrames=True&SubtitleStreamIndexes=-1&ManifestSubtitles=vtt&h264-profile=high,main,baseline,constrainedbaseline,high10&h264-level=62&hevc-codectag=hvc1,hev1,hevc,hdmv&TranscodeReasons=ContainerBitrateExceedsLimit",
            "AddApiKeyToDirectStreamUrl": false,
            "TranscodingUrl": "/videos/61162/master.m3u8?DeviceId=e12ab84a-2948-463a-9cb0-ecebcac2949e&MediaSourceId=37bcb3827aeb2095f1b6293e9a189072&PlaySessionId=e643532a23ca4a60b2c70d4210c018a9&api_key=e824c422e02047dfa820dbcab5490bb9&VideoCodec=h264,h265,hevc,av1&VideoBitrate=1400000&TranscodingMaxAudioChannels=2&SegmentContainer=ts&MinSegments=1&BreakOnNonKeyFrames=True&SubtitleStreamIndexes=-1&ManifestSubtitles=vtt&h264-profile=high,main,baseline,constrainedbaseline,high10&h264-level=62&hevc-codectag=hvc1,hev1,hevc,hdmv&TranscodeReasons=ContainerBitrateExceedsLimit",
            "TranscodingSubProtocol": "hls",
            "TranscodingContainer": "ts",
            "ReadAtNativeFramerate": false,
            "ItemId": "61162"
        }
    ],
    "PlaySessionId": "e643532a23ca4a60b2c70d4210c018a9"
}
```
普通视频
```
{
    "MediaSources": [
        {
            "Protocol": "File",
            "Id": "56f692be38b2c4ae3c8f155e77006d46",
            "Path": "/media/媒体库/动漫追番/青之箱 (2024)/Season 1/青之箱 S01E04  -HEVC -AAC -KitaujiSub.mkv",
            "Type": "Default",
            "Container": "mkv",
            "Size": 470388402,
            "Name": "青之箱 S01E04  -HEVC -AAC -KitaujiSub",
            "IsRemote": false,
            "HasMixedProtocols": false,
            "RunTimeTicks": 14172790000,
            "SupportsTranscoding": true,
            "SupportsDirectStream": false,
            "SupportsDirectPlay": false,
            "IsInfiniteStream": false,
            "RequiresOpening": false,
            "RequiresClosing": false,
            "RequiresLooping": false,
            "SupportsProbing": false,
            "MediaStreams": [
                {
                    "Codec": "hevc",
                    "ColorSpace": "bt709",
                    "TimeBase": "1/1000",
                    "VideoRange": "SDR",
                    "DisplayTitle": "1080p HEVC",
                    "IsInterlaced": false,
                    "BitRate": 2655163,
                    "BitDepth": 10,
                    "RefFrames": 1,
                    "IsDefault": true,
                    "IsForced": false,
                    "IsHearingImpaired": false,
                    "Height": 1080,
                    "Width": 1920,
                    "AverageFrameRate": 23.976025,
                    "RealFrameRate": 23.976025,
                    "Profile": "Main 10",
                    "Type": "Video",
                    "AspectRatio": "16:9",
                    "Index": 0,
                    "IsExternal": false,
                    "IsTextSubtitleStream": false,
                    "SupportsExternalStream": false,
                    "Protocol": "File",
                    "PixelFormat": "yuv420p10le",
                    "Level": 120,
                    "IsAnamorphic": false,
                    "ExtendedVideoType": "None",
                    "ExtendedVideoSubType": "None",
                    "ExtendedVideoSubTypeDescription": "None",
                    "AttachmentSize": 0
                },
                {
                    "Codec": "aac",
                    "Language": "jpn",
                    "TimeBase": "1/1000",
                    "DisplayTitle": "Japanese AAC 5.1 (默认)",
                    "DisplayLanguage": "Japanese",
                    "IsInterlaced": false,
                    "ChannelLayout": "5.1",
                    "BitRate": 320000,
                    "Channels": 6,
                    "SampleRate": 48000,
                    "IsDefault": true,
                    "IsForced": false,
                    "IsHearingImpaired": false,
                    "Profile": "LC",
                    "Type": "Audio",
                    "Index": 1,
                    "IsExternal": false,
                    "IsTextSubtitleStream": false,
                    "SupportsExternalStream": false,
                    "Protocol": "File",
                    "ExtendedVideoType": "None",
                    "ExtendedVideoSubType": "None",
                    "ExtendedVideoSubTypeDescription": "None",
                    "AttachmentSize": 0
                },
                {
                    "Codec": "ass",
                    "Language": "chi",
                    "TimeBase": "1/1000",
                    "Title": "chs_jp",
                    "DisplayTitle": "Chinese Simplified (默认 ASS)",
                    "DisplayLanguage": "Chinese Simplified",
                    "IsInterlaced": false,
                    "IsDefault": true,
                    "IsForced": false,
                    "IsHearingImpaired": false,
                    "Type": "Subtitle",
                    "Index": 2,
                    "IsExternal": false,
                    "DeliveryMethod": "External",
                    "DeliveryUrl": "/Videos/61256/56f692be38b2c4ae3c8f155e77006d46/Subtitles/2/0/Stream.ass?api_key=e824c422e02047dfa820dbcab5490bb9",
                    "IsExternalUrl": false,
                    "IsTextSubtitleStream": true,
                    "SupportsExternalStream": true,
                    "Protocol": "File",
                    "ExtendedVideoType": "None",
                    "ExtendedVideoSubType": "None",
                    "ExtendedVideoSubTypeDescription": "None",
                    "AttachmentSize": 0,
                    "SubtitleLocationType": "InternalStream"
                },
                {
                    "Codec": "ass",
                    "Language": "chi",
                    "TimeBase": "1/1000",
                    "Title": "cht_jp",
                    "DisplayTitle": "Chinese Simplified (ASS)",
                    "DisplayLanguage": "Chinese Simplified",
                    "IsInterlaced": false,
                    "IsDefault": false,
                    "IsForced": false,
                    "IsHearingImpaired": false,
                    "Type": "Subtitle",
                    "Index": 3,
                    "IsExternal": false,
                    "DeliveryMethod": "External",
                    "DeliveryUrl": "/Videos/61256/56f692be38b2c4ae3c8f155e77006d46/Subtitles/3/0/Stream.ass?api_key=e824c422e02047dfa820dbcab5490bb9",
                    "IsExternalUrl": false,
                    "IsTextSubtitleStream": true,
                    "SupportsExternalStream": true,
                    "Protocol": "File",
                    "ExtendedVideoType": "None",
                    "ExtendedVideoSubType": "None",
                    "ExtendedVideoSubTypeDescription": "None",
                    "AttachmentSize": 0,
                    "SubtitleLocationType": "InternalStream"
                },
                {
                    "Codec": "otf",
                    "TimeBase": "1/90000",
                    "IsInterlaced": false,
                    "IsDefault": false,
                    "IsForced": false,
                    "IsHearingImpaired": false,
                    "Type": "Attachment",
                    "Index": 4,
                    "IsExternal": false,
                    "IsTextSubtitleStream": false,
                    "SupportsExternalStream": false,
                    "Path": "FOT-ModeMinBLargeStd-B.0.E8KEG6LU.otf",
                    "Protocol": "File",
                    "ExtendedVideoType": "None",
                    "ExtendedVideoSubType": "None",
                    "ExtendedVideoSubTypeDescription": "None",
                    "AttachmentSize": 49752,
                    "MimeType": "application/vnd.ms-opentype"
                },
                {
                    "Codec": "otf",
                    "TimeBase": "1/90000",
                    "IsInterlaced": false,
                    "IsDefault": false,
                    "IsForced": false,
                    "IsHearingImpaired": false,
                    "Type": "Attachment",
                    "Index": 5,
                    "IsExternal": false,
                    "IsTextSubtitleStream": false,
                    "SupportsExternalStream": false,
                    "Path": "FOT-SeuratProN-DB.0.JIIL4FRM.otf",
                    "Protocol": "File",
                    "ExtendedVideoType": "None",
                    "ExtendedVideoSubType": "None",
                    "ExtendedVideoSubTypeDescription": "None",
                    "AttachmentSize": 196388,
                    "MimeType": "application/vnd.ms-opentype"
                },
                {
                    "Codec": "otf",
                    "TimeBase": "1/90000",
                    "IsInterlaced": false,
                    "IsDefault": false,
                    "IsForced": false,
                    "IsHearingImpaired": false,
                    "Type": "Attachment",
                    "Index": 6,
                    "IsExternal": false,
                    "IsTextSubtitleStream": false,
                    "SupportsExternalStream": false,
                    "Path": "FOT-TsukuMinPr5N-B.0.8X1XX3RE.otf",
                    "Protocol": "File",
                    "ExtendedVideoType": "None",
                    "ExtendedVideoSubType": "None",
                    "ExtendedVideoSubTypeDescription": "None",
                    "AttachmentSize": 82360,
                    "MimeType": "application/vnd.ms-opentype"
                },
                {
                    "Codec": "ttf",
                    "TimeBase": "1/90000",
                    "IsInterlaced": false,
                    "IsDefault": false,
                    "IsForced": false,
                    "IsHearingImpaired": false,
                    "Type": "Attachment",
                    "Index": 7,
                    "IsExternal": false,
                    "IsTextSubtitleStream": false,
                    "SupportsExternalStream": false,
                    "Path": "FZLANTINGHEI-DB-GBK.0.WOAP9IVQ.ttf",
                    "Protocol": "File",
                    "ExtendedVideoType": "None",
                    "ExtendedVideoSubType": "None",
                    "ExtendedVideoSubTypeDescription": "None",
                    "AttachmentSize": 15908,
                    "MimeType": "application/x-truetype-font"
                },
                {
                    "Codec": "ttf",
                    "TimeBase": "1/90000",
                    "IsInterlaced": false,
                    "IsDefault": false,
                    "IsForced": false,
                    "IsHearingImpaired": false,
                    "Type": "Attachment",
                    "Index": 8,
                    "IsExternal": false,
                    "IsTextSubtitleStream": false,
                    "SupportsExternalStream": false,
                    "Path": "方正粗雅宋_GBK.0.ELFFB6DY.ttf",
                    "Protocol": "File",
                    "ExtendedVideoType": "None",
                    "ExtendedVideoSubType": "None",
                    "ExtendedVideoSubTypeDescription": "None",
                    "AttachmentSize": 89012,
                    "MimeType": "application/x-truetype-font"
                },
                {
                    "Codec": "ttf",
                    "TimeBase": "1/90000",
                    "IsInterlaced": false,
                    "IsDefault": false,
                    "IsForced": false,
                    "IsHearingImpaired": false,
                    "Type": "Attachment",
                    "Index": 9,
                    "IsExternal": false,
                    "IsTextSubtitleStream": false,
                    "SupportsExternalStream": false,
                    "Path": "方正兰亭大黑_GBK.0.5PPVFM7L.ttf",
                    "Protocol": "File",
                    "ExtendedVideoType": "None",
                    "ExtendedVideoSubType": "None",
                    "ExtendedVideoSubTypeDescription": "None",
                    "AttachmentSize": 13900,
                    "MimeType": "application/x-truetype-font"
                },
                {
                    "Codec": "ttf",
                    "TimeBase": "1/90000",
                    "IsInterlaced": false,
                    "IsDefault": false,
                    "IsForced": false,
                    "IsHearingImpaired": false,
                    "Type": "Attachment",
                    "Index": 10,
                    "IsExternal": false,
                    "IsTextSubtitleStream": false,
                    "SupportsExternalStream": false,
                    "Path": "方正兰亭黑_GBK.0.OAU1SQQO.ttf",
                    "Protocol": "File",
                    "ExtendedVideoType": "None",
                    "ExtendedVideoSubType": "None",
                    "ExtendedVideoSubTypeDescription": "None",
                    "AttachmentSize": 33180,
                    "MimeType": "application/x-truetype-font"
                },
                {
                    "Codec": "ttf",
                    "TimeBase": "1/90000",
                    "IsInterlaced": false,
                    "IsDefault": false,
                    "IsForced": false,
                    "IsHearingImpaired": false,
                    "Type": "Attachment",
                    "Index": 11,
                    "IsExternal": false,
                    "IsTextSubtitleStream": false,
                    "SupportsExternalStream": false,
                    "Path": "方正兰亭圆_GBK_准.0.D3ABRYMS.ttf",
                    "Protocol": "File",
                    "ExtendedVideoType": "None",
                    "ExtendedVideoSubType": "None",
                    "ExtendedVideoSubTypeDescription": "None",
                    "AttachmentSize": 354176,
                    "MimeType": "application/x-truetype-font"
                },
                {
                    "Codec": "ttf",
                    "TimeBase": "1/90000",
                    "IsInterlaced": false,
                    "IsDefault": false,
                    "IsForced": false,
                    "IsHearingImpaired": false,
                    "Type": "Attachment",
                    "Index": 12,
                    "IsExternal": false,
                    "IsTextSubtitleStream": false,
                    "SupportsExternalStream": false,
                    "Path": "方正准雅宋_GBK.0.T77HXJCC.ttf",
                    "Protocol": "File",
                    "ExtendedVideoType": "None",
                    "ExtendedVideoSubType": "None",
                    "ExtendedVideoSubTypeDescription": "None",
                    "AttachmentSize": 69468,
                    "MimeType": "application/x-truetype-font"
                },
                {
                    "Codec": "ttf",
                    "TimeBase": "1/90000",
                    "IsInterlaced": false,
                    "IsDefault": false,
                    "IsForced": false,
                    "IsHearingImpaired": false,
                    "Type": "Attachment",
                    "Index": 13,
                    "IsExternal": false,
                    "IsTextSubtitleStream": false,
                    "SupportsExternalStream": false,
                    "Path": "华康翩翩体W3-A.0.B7WYAJQK.ttf",
                    "Protocol": "File",
                    "ExtendedVideoType": "None",
                    "ExtendedVideoSubType": "None",
                    "ExtendedVideoSubTypeDescription": "None",
                    "AttachmentSize": 29816,
                    "MimeType": "application/x-truetype-font"
                },
                {
                    "Codec": "ttf",
                    "TimeBase": "1/90000",
                    "IsInterlaced": false,
                    "IsDefault": false,
                    "IsForced": false,
                    "IsHearingImpaired": false,
                    "Type": "Attachment",
                    "Index": 14,
                    "IsExternal": false,
                    "IsTextSubtitleStream": false,
                    "SupportsExternalStream": false,
                    "Path": "华康翩翩体W5-A.0.CVH5DGCU.ttf",
                    "Protocol": "File",
                    "ExtendedVideoType": "None",
                    "ExtendedVideoSubType": "None",
                    "ExtendedVideoSubTypeDescription": "None",
                    "AttachmentSize": 58720,
                    "MimeType": "application/x-truetype-font"
                },
                {
                    "Codec": "ttf",
                    "TimeBase": "1/90000",
                    "IsInterlaced": false,
                    "IsDefault": false,
                    "IsForced": false,
                    "IsHearingImpaired": false,
                    "Type": "Attachment",
                    "Index": 15,
                    "IsExternal": false,
                    "IsTextSubtitleStream": false,
                    "SupportsExternalStream": false,
                    "Path": "华康手札体W5-A.0.B8DWXF5W.ttf",
                    "Protocol": "File",
                    "ExtendedVideoType": "None",
                    "ExtendedVideoSubType": "None",
                    "ExtendedVideoSubTypeDescription": "None",
                    "AttachmentSize": 56332,
                    "MimeType": "application/x-truetype-font"
                }
            ],
            "Formats": [],
            "Bitrate": 2655163,
            "RequiredHttpHeaders": {},
            "DirectStreamUrl": "/videos/61256/master.m3u8?DeviceId=e12ab84a-2948-463a-9cb0-ecebcac2949e&MediaSourceId=56f692be38b2c4ae3c8f155e77006d46&PlaySessionId=592ccdf484644f3db25993c423f392ad&api_key=e824c422e02047dfa820dbcab5490bb9&VideoCodec=h264,h265,hevc,av1&AudioCodec=ac3,mp3,aac&VideoBitrate=1430000&AudioBitrate=320000&AudioStreamIndex=1&TranscodingMaxAudioChannels=2&SegmentContainer=ts&MinSegments=1&BreakOnNonKeyFrames=True&SubtitleStreamIndexes=-1&ManifestSubtitles=vtt&h264-profile=high,main,baseline,constrainedbaseline,high10&h264-level=62&hevc-codectag=hvc1,hev1,hevc,hdmv&TranscodeReasons=ContainerBitrateExceedsLimit",
            "AddApiKeyToDirectStreamUrl": false,
            "TranscodingUrl": "/videos/61256/master.m3u8?DeviceId=e12ab84a-2948-463a-9cb0-ecebcac2949e&MediaSourceId=56f692be38b2c4ae3c8f155e77006d46&PlaySessionId=592ccdf484644f3db25993c423f392ad&api_key=e824c422e02047dfa820dbcab5490bb9&VideoCodec=h264,h265,hevc,av1&AudioCodec=ac3,mp3,aac&VideoBitrate=1430000&AudioBitrate=320000&AudioStreamIndex=1&TranscodingMaxAudioChannels=2&SegmentContainer=ts&MinSegments=1&BreakOnNonKeyFrames=True&SubtitleStreamIndexes=-1&ManifestSubtitles=vtt&h264-profile=high,main,baseline,constrainedbaseline,high10&h264-level=62&hevc-codectag=hvc1,hev1,hevc,hdmv&TranscodeReasons=ContainerBitrateExceedsLimit",
            "TranscodingSubProtocol": "hls",
            "TranscodingContainer": "ts",
            "ReadAtNativeFramerate": false,
            "DefaultAudioStreamIndex": 1,
            "DefaultSubtitleStreamIndex": 2,
            "ItemId": "61256"
        }
    ],
    "PlaySessionId": "592ccdf484644f3db25993c423f392ad"
}
```