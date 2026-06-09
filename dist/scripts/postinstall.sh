#!/bin/bash
# postinstall.sh - 安装后脚本

# 创建数据目录
mkdir -p /var/lib/block-area-bot
mkdir -p /var/log/block-area-bot

# 重新加载 systemd
systemctl daemon-reload

echo "Block Area Bot 安装完成"
echo "使用 'block enable && block start' 启动服务"
