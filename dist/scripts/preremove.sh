#!/bin/bash
# preremove.sh - 卸载前脚本

# 停止并禁用服务
systemctl stop block-area-bot 2>/dev/null || true
systemctl disable block-area-bot 2>/dev/null || true

echo "Block Area Bot 服务已停止"
