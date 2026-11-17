# SimCallHub

> 🚀 **4G 模块全能智能电话网关**：在 100% 保留大模型短信智能分类、脱敏推送与 AI 代接的基础上，**原生内嵌纯 Go 实现的轻量 SIP VoIP 分机服务与 SGP.22 eSIM 配置文件管理**。
>
> 告别庞大臃肿的 Asterisk + FreePBX + MySQL（无需动辄 2GB+ 内存与复杂配置），仅需单个极简容器，即可让普通 4G 模块（如移远 EC20/EC25）化身为企业级智能 VoIP 电话交换机！

---

## ✨ 核心特性

- 📱 **双模通话（SIP 客户端 + 浏览器 WebRTC）**：
  - **原生内嵌 SIP 服务器**：支持 iOS（Groundwire / Zoiper）、Windows（MicroSIP）、Android（各类 SIP 软电话）以及硬件 IP 电话注册。
  - **手机锁屏接听**：来电时，手机 App 像普通手机一样在锁屏状态下振铃并接听，通话清晰流畅。
  - **SIP 软电话外呼**：直接在拨号盘输入目标手机号码呼出，通过 4G 卡拨打电话。
  - **原生网页通话保留**：依然可在浏览器后台直接打接电话。
- 💳 **VoHive 同款 eSIM / eUICC 配置文件管理**：
  - **芯片与 EID 识别**：自动通过 SGP.22 / 3GPP AT 逻辑通道识别卡槽是否为 eUICC 芯片并读取 32 位 EID。
  - **配置文件列表**：清晰列出所有已下载的运营商 Profile（ICCID、运营商名称、自定义备注、激活状态）。
  - **一键切换与管理**：支持在线一键启用/切换当前活动配置（自动软重启模组重新搜网）、停用配置、修改卡备注名称及删除闲置配置。
  - **硬件广泛兼容**：支持焊有 eSIM 芯片的 EC20 开发板，以及 5ber、estk.me、sysmocom 等可写实体 eSIM 实体卡。
- 🤖 **AI 协同接听**：
  - 来电时优先振铃你的 SIP 手机客户端；
  - 超过设定时间（默认 20 秒）未接听或手动拒接，自动无缝转由 **AI 助手（阿里百炼大模型）代接**，询问对方事由并生成文字摘要推送到手机！
- ✉️ **大模型短信智能处理**：
  - 自动识别验证码、快递、账单、营销推广；
  - 本地脱敏后由大模型（DeepSeek、通义千问等）分类，支持微信/飞书/钉钉/Telegram 推送。
- ⚡ **libusb 免驱直连**：
  - 群晖 DSM 7.x（黑群晖）即使缺少 Linux 内核 `option` 串口驱动，也能通过 USB 底层总线直接驱动 EC20。
- 🪶 **极度轻量**：
  - 纯 Go 原生实现（嵌入 G.711u/a 转码与 RTP 媒体引擎），镜像体积极小，运行内存仅 **几十 MB**。

---

## 🛠️ SIP 客户端配置说明

部署运行后，SIP 服务默认监听在 UDP `5060` 端口。在手机或电脑打开任意 SIP 客户端（如 **Groundwire**、**Zoiper** 或 **MicroSIP**），添加账号：

| 配置项 | 填写内容 | 说明 |
| :--- | :--- | :--- |
| **用户名 / 账号 (User / Extension)** | `1001` | 默认分机号（可在环境变量修改） |
| **域名 / 服务器 (Domain / Server)** | `你的群晖或服务器IP` | 端口填 `5060`，协议选 **UDP** |
| **密码 (Password)** | `12345678` | 默认密码（可在环境变量修改） |
| **传输协议 (Transport)** | `UDP` | 强烈建议使用 UDP |
| **音频编码 (Audio Codecs)** | `PCMU (G.711u)` 或 `PCMA (G.711a)` | 8kHz 采样率，原生直通 |

---

## 🐳 Docker 快速部署

### 1. 使用 docker-compose 启动

创建 `docker-compose.yml`：

```yaml
services:
  SimCallHub:
    image: chenpinetree/SimCallHub:latest
    container_name: SimCallHub
    restart: unless-stopped
    privileged: true
    devices:
      - /dev/bus/usb:/dev/bus/usb
    volumes:
      - ./data:/data
    ports:
      # Web 管理后台端口
      - "9081:9081"
      # SIP VoIP 分机注册与信令端口 (UDP)
      - "5060:5060/udp"
      # RTP 通话音频媒体流端口 (UDP)
      - "20000-20100:20000-20100/udp"
    environment:
      - TZ=Asia/Shanghai
      - HAOBOT_DATA=/data
      - SIM_SIP_USER=1001          # 你的分机号
      - SIM_SIP_PASS=12345678      # 你的分机密码
      - SIM_SIP_LISTEN=0.0.0.0:5060
    command: ["serve", "-listen", "0.0.0.0", "-transport", "usb"]
```

启动命令：
```bash
docker-compose up -d
docker logs -f SimCallHub
```

---

## ⚙️ 环境变量与参数说明

| 环境变量 / 命令行参数 | 默认值 | 说明 |
| :--- | :--- | :--- |
| `SIM_SIP_LISTEN` / `-sip-listen` | `0.0.0.0:5060` | SIP 服务器监听地址与 UDP 端口 |
| `SIM_SIP_USER` / `-sip-user` | `1001` | 注册分机号 |
| `SIM_SIP_PASS` / `-sip-pass` | `12345678` | 分机密码 |
| `-sip-rtp-start` | `20000` | RTP 媒体起始端口 |
| `-sip-rtp-end` | `20100` | RTP 媒体结束端口 |
| `-sip-timeout` | `20` | 分机振铃超时秒数（超时后转交 AI 自动代接） |
| `HAOBOT_DATA` | `/data` | 数据库与录音文件存储目录 |

---

## 📦 本地开发与编译

本项目基于 Go 1.24+ 编写，包含完整的前端 React 管理界面（预编译嵌入在程序中）。

```bash
# 本地编译测试
go test ./internal/agent/sip/...
go build -o bin/SimCallHub ./cmd/haobot-agent

# 构建跨平台 Linux Docker 镜像
docker build --platform linux/amd64 -t SimCallHub:local .
```

---

## 📄 开源许可

本项目遵循 [MIT License](LICENSE)。
