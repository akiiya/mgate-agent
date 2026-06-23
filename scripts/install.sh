#!/bin/sh
# v0.1.0 RC 安装脚本：只安装二进制、目录和 systemd unit，不覆盖用户配置和凭证。
set -eu

if [ "$(id -u)" -ne 0 ]; then
  echo "请使用 root 运行安装脚本。" >&2
  exit 1
fi

script_dir=$(CDPATH= cd "$(dirname "$0")" && pwd)
root_dir=$(CDPATH= cd "$script_dir/.." && pwd)

bin_src="${1:-}"
if [ -z "$bin_src" ]; then
  if [ -f "$root_dir/mgate-agent" ]; then
    bin_src="$root_dir/mgate-agent"
  elif [ -f "$root_dir/bin/mgate-agent" ]; then
    bin_src="$root_dir/bin/mgate-agent"
  else
    echo "未找到 mgate-agent 二进制。请先执行 make build，或把二进制路径作为第一个参数传入。" >&2
    exit 1
  fi
fi

if [ ! -f "$bin_src" ]; then
  echo "二进制不存在: $bin_src" >&2
  exit 1
fi

install -m 0755 "$bin_src" /usr/local/bin/mgate-agent

install -d -m 0755 /etc/mgate-agent
install -d -m 0700 /var/lib/mgate-agent
install -d -m 0700 /var/lib/mgate-agent/outbox
install -d -m 0755 /var/log/mgate-agent

if [ ! -f /etc/mgate-agent/agent.yaml ]; then
  if [ -f "$root_dir/configs/agent.example.yaml" ]; then
    install -m 0644 "$root_dir/configs/agent.example.yaml" /etc/mgate-agent/agent.yaml
    echo "已安装示例配置: /etc/mgate-agent/agent.yaml"
  else
    echo "未找到 configs/agent.example.yaml，请稍后用 mgate-agent config default 生成配置。" >&2
  fi
else
  echo "保留已有配置: /etc/mgate-agent/agent.yaml"
fi

if [ -f "$root_dir/packaging/systemd/mgate-agent.service" ]; then
  install -m 0644 "$root_dir/packaging/systemd/mgate-agent.service" /etc/systemd/system/mgate-agent.service
  echo "已安装 systemd unit: /etc/systemd/system/mgate-agent.service"
fi

if command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload
  echo "已执行 systemctl daemon-reload"
fi

echo "请手动准备凭证文件: /var/lib/mgate-agent/credentials.json"
echo "凭证文件仍为 JSON，Linux 权限必须是 0600。"
echo "安装脚本不会生成 credentials，也不会输出任何 secret。"
echo "建议先运行: mgate-agent check --config /etc/mgate-agent/agent.yaml"
echo "确认通过后再手动启动: systemctl start mgate-agent"
