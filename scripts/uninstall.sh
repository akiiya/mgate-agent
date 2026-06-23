#!/bin/sh
# v0.1.0 RC 卸载脚本：只移除服务和二进制，默认保留配置、凭证、日志和 outbox。
set -eu

if [ "$(id -u)" -ne 0 ]; then
  echo "请使用 root 运行卸载脚本。" >&2
  exit 1
fi

if command -v systemctl >/dev/null 2>&1; then
  systemctl stop mgate-agent.service 2>/dev/null || true
  systemctl disable mgate-agent.service 2>/dev/null || true
fi

rm -f /usr/local/bin/mgate-agent
rm -f /etc/systemd/system/mgate-agent.service

if command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload
fi

echo "已移除 mgate-agent 服务文件和二进制。"
echo "默认保留以下目录，避免误删配置、credentials、outbox 和审计日志："
echo "  /etc/mgate-agent"
echo "  /var/lib/mgate-agent"
echo "  /var/log/mgate-agent"
echo "如确需删除数据，请确认已备份后手动处理。"
