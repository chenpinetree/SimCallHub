# SimCallHub
# 集成原生 Web 管理、大模型短信分类、AI代接，以及轻量 SIP VoIP 分机服务
# 适用于群晖 NAS / 飞牛 NAS / Linux 服务器

FROM --platform=linux/amd64 golang:1.25-bookworm AS build

ENV GOPROXY=https://goproxy.cn,direct \
    CGO_ENABLED=1

RUN apt-get update \
 && apt-get install -y --no-install-recommends libusb-1.0-0-dev pkg-config git ca-certificates \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY . .
RUN go build -trimpath -ldflags "-s -w" -o /out/haobot-agent ./cmd/haobot-agent

# ---------- 运行阶段 ----------
FROM --platform=linux/amd64 debian:bookworm-slim

RUN apt-get update \
 && apt-get install -y --no-install-recommends libusb-1.0-0 ca-certificates tzdata usbutils \
 && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/haobot-agent /usr/local/bin/haobot-agent

ENV TZ=Asia/Shanghai \
    HAOBOT_DATA=/data \
    HAOBOT_PORT=9081 \
    SIM_SIP_LISTEN=0.0.0.0:5060 \
    SIM_SIP_USER=1001 \
    SIM_SIP_PASS=12345678

VOLUME ["/data"]

# 9081: Web 后台 (HTTPS)
# 5060/udp: SIP 信令
# 20000-20100/udp: RTP 语音媒体流
EXPOSE 9081 5060/udp 20000-20100/udp

ENTRYPOINT ["haobot-agent"]
CMD ["serve", "-listen", "0.0.0.0", "-transport", "usb"]
