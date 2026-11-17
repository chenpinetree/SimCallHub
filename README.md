# SimCallHub

<p align="center">
  <img src="docs/images/promo-hero.jpg" alt="SimCallHub Hero" width="800" style="border-radius: 8px;" />
</p>

<p align="center">
  <strong>基于 4G/5G 蜂窝模组的全能智能电话网关与短信中枢</strong><br>
  集成 SIP VoIP 电话分机 + eSIM/eUICC 配置文件管理 + 大模型短信脱敏分类 + AI 语音代接 + Web 管理后台
</p>

<p align="center">
  <a href="#-核心特性">核心特性</a> •
  <a href="#-支持硬件与系统">支持硬件</a> •
  <a href="#-功能详解">功能详解</a> •
  <a href="#-快速部署指南">快速部署</a> •
  <a href="#-sip-客户端配置">SIP 配置</a> •
  <a href="#-常见问题-faq">FAQ</a>
</p>

---

## 💡 为什么开发 SimCallHub？

传统在 NAS（群晖、飞牛）或软路由上使用 4G 模组（如 EC20）接打电话，通常需要部署 **MySQL 8.4 + Asterisk + FreePBX** 这一整套企业级交换机系统：
- 内存开销动辄 **2GB~4GB+**，启动缓慢且配置极其晦涩；
- 强依赖 Linux 内核 `option` 串口驱动，在**黑群晖 DSM 7.x**（官方移除了 `option.ko`）上极难跑通；
- 没有大模型短信智能提取能力，也无法在手机没空接听时让 AI 代接。

**SimCallHub 彻底重构了这一流程**：
采用纯 Go 原生实现，仅需**单个几十 MB 的轻量 Docker 容器**，即可将普通 4G/5G 模组变成你私人专属的智能电话局！

---

## ✨ 核心特性

- 📱 **SIP VoIP 电话分机（软电话接打）**：
  - 内置纯 Go 实现的 RFC 3261 SIP 服务器与 RFC 3550 RTP 媒体引擎；
  - 手机客户端（iOS **Groundwire** / **Zoiper**、Windows **MicroSIP**、Android）后台常驻，来电在锁屏下直接像接真实电话一样振铃、高清接听；
  - 可以在软电话拨号盘直接拨打任意真实手机号呼出。
- 💳 **VoHive 同款 SGP.22 eSIM 配置文件管理**：
  - 自动通过 3GPP AT 逻辑通道（`AT+CCHO`/`AT+CGLA`）识别 eUICC 芯片并读取 32 位 EID；
  - 网页上一键列出芯片内所有运营商 Profile，**支持一键在线切换生效卡号**、停用配置、修改备注名称及删除闲置配置。
- 🤖 **AI 大模型协同接听（百炼语音大模型）**：
  - 来电时优先振铃你的手机 SIP 分机；
  - 超过设定时间（默认 20 秒）未接听或手动拒接，自动无缝转由 **AI 助手实时语音代接**，询问来电者身份及事由，并生成文字总结摘要推送到你的微信/飞书！
- ✉️ **大模型短信智能脱敏与分类**：
  - 新短信先在**本地全自动脱敏**（敏感信息、金额、密码掩码），再由大模型（通义千问、DeepSeek 等）智能归类为验证码、快递、账单、营销通知；
  - 一键复制提取验证码，支持微信（企业微信）、飞书、钉钉、Telegram 实时推送。
- ⚡ **libusb 免驱直连（完美适配黑群晖 DSM 7.x）**：
  - 绕过 Linux 内核缺失 `option.ko` 的限制，直接通过 USB 底层总线接管 EC20，即插即用。
- 🪶 **极度轻量低碳**：
  - 静态编译，零外部数据库依赖（内置 pure-Go SQLite），运行内存仅 **数十 MB**。

---

## 🖥️ 支持硬件与系统

### 1. 蜂窝模组（Cellular Modem）
| 厂商 | 型号 | 验证状态 | 说明 |
| :--- | :--- | :--- | :--- |
| **移远 (Quectel)** | **EC20 / EC25 / EG25** | ✅ **真机深度验证（强烈推荐）** | 支持语音、UAC 音频、SMS、eSIM |
| **移远 (Quectel)** | **EC21 / EG91 / EG95 / BG96** | ✅ 协议支持 | 移远 AT 协议家族兼容 |
| **芯讯通 (SIMCom)** | **SIM7600 系列** | ⚠️ 基础支持 | 支持 AT 控制与短信 |
| **高通方案棒子** | **Qualcomm 410 / 4G 随身 WiFi** | ⚠️ 社区适配中 | 需切为 Modem 模式 |

### 2. eSIM / eUICC 芯片支持
- 板载/焊接有 eUICC 芯片的 4G 模组开发板；
- 实体可写 eSIM 卡：**5ber**、**estk.me**、**sysmocom** 等（插入卡槽即可在线读写 Profile）。

### 3. 宿主系统（Host OS）
- **群晖 Synology DSM**：DSM 6.2 / 7.0 / 7.1 / 7.2 / 7.2.2（白群晖、黑群晖 DS920+ 等全系列）；
- **飞牛 NAS (fnOS)**；
- **Linux 64 位发行版**：Debian / Ubuntu / Alpine / CentOS / OpenWrt 软路由；
- **macOS**：Apple Silicon (M系列) / Intel 架构原生支持。

---

## 🐳 快速部署指南

### 1. Docker Compose 一键启动（推荐）

在你的服务器或群晖 NAS 上创建目录并新建 `docker-compose.yml`：

```yaml
services:
  simcallhub:
    image: chenpinetree/simcallhub:latest
    container_name: simcallhub
    restart: unless-stopped
    privileged: true
    devices:
      - /dev/bus/usb:/dev/bus/usb
    volumes:
      - ./data:/data
    ports:
      # Web 管理后台端口 (HTTPS)
      - "9081:9081"
      # SIP VoIP 分机信令端口 (UDP)
      - "5060:5060/udp"
      # RTP 实时通话媒体流端口范围 (UDP)
      - "20000-20100:20000-20100/udp"
    environment:
      - TZ=Asia/Shanghai
      - SIMCALLHUB_DATA=/data
      - SIM_SIP_USER=1001          # SIP 分机号
      - SIM_SIP_PASS=12345678      # SIP 分机登录密码
      - SIM_SIP_LISTEN=0.0.0.0:5060
    command: ["serve", "-listen", "0.0.0.0", "-transport", "usb"]
```

运行启动命令：
```bash
docker-compose up -d
docker logs -f simcallhub
```

> **首次登录**：启动后查看日志，会显示随机生成的 `admin` 密码。在浏览器访问 `https://NAS_IP:9081`（由于使用自签名证书，点击“高级 → 继续访问”即可）。

---

## 🛠️ SIP 客户端配置

启动后，在手机或电脑下载任意标准 SIP 客户端，填写分机参数即可连通 4G 卡：

### 推荐客户端
- **iPhone / iOS**：首选 **Groundwire**（支持高可靠后台 Push 唤醒）或免费的 **Zoiper**；
- **Windows PC**：推荐 **MicroSIP**（轻巧开源）；
- **Android**：**Groundwire (Acrobits)**、**Zoiper** 或开源 SIP 客户端。

### 填写参数一览
| 配置项 | 填写内容 | 说明 |
| :--- | :--- | :--- |
| **用户名 / 账号 (User / Extension)** | `1001` | 默认分机号（由 `SIM_SIP_USER` 设定） |
| **域名 / 服务器 (Domain / Server)** | `你的群晖或服务器IP:5060` | 协议必须为 **UDP** |
| **密码 (Password)** | `12345678` | 默认密码（由 `SIM_SIP_PASS` 设定） |
| **音频编码 (Codecs)** | 勾选 `PCMU (G.711u)` 或 `PCMA (G.711a)` | 8kHz 原生直通，超低延迟 |

---

## ⚙️ 环境变量与参数表

| 环境变量 / 参数 | 默认值 | 说明 |
| :--- | :--- | :--- |
| `SIM_SIP_LISTEN` / `-sip-listen` | `0.0.0.0:5060` | SIP 服务器监听地址与 UDP 端口 |
| `SIM_SIP_USER` / `-sip-user` | `1001` | 默认分机账号 |
| `SIM_SIP_PASS` / `-sip-pass` | `12345678` | 分机连接密码 |
| `-sip-rtp-start` | `20000` | RTP 通话媒体流起始端口 |
| `-sip-rtp-end` | `20100` | RTP 通话媒体流结束端口 |
| `-sip-timeout` | `20` | 分机振铃超时秒数（超时后自动切入 AI 接听） |
| `SIMCALLHUB_DATA` / `-data` | `/data` | 数据库、录音及密钥存储目录 |

---

## ❓ 常见问题 (FAQ)

#### Q1: 黑群晖没有 `/dev/ttyUSB*` 串口节点可以运行吗？
**A**: 完全可以！SimCallHub 内置了 libusb 驱动层，指定 `-transport usb` 时，容器直接挂载 `/dev/bus/usb` 就能接管 EC20，根本不需要群晖内核存在 `option.ko`。

#### Q2: 呼叫转移与 AI 代接如何配置？
**A**: 登录后台后，在“设置 → 消息推送”填入你的飞书/微信 Webhook，在“设置 → AI 接听”填入阿里百炼（Model Studio）API Key。当手机未接听时，AI 助手会自动接起并把对话总结推送到你的手机。

#### Q3: 如何管理我的 eSIM 卡？
**A**: 在 Web 后台切换到“设备 → eSIM”选项卡，系统会自动读取 eUICC 芯片并展示所有可用 Profile。点击任意卡片旁的“启用并切换”，模组会自动完成重新注册。

---

## 📄 开源许可

本项目遵循 [MIT License](LICENSE)。欢迎提交 Issue 与 Pull Request！
