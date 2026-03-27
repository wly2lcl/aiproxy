#!/bin/sh
set -e

DATA_DIR="${DATA_DIR:-/app/data}"
CONFIG_DIR="${CONFIG_DIR:-/app/config}"

if [ "$(id -u)" = "0" ]; then
    echo "Running as root, fixing permissions..."
    
    # 修复数据目录权限（需要写入）
    if [ -d "$DATA_DIR" ]; then
        chown -R appuser:appuser "$DATA_DIR" 2>/dev/null || true
        echo "Fixed permissions for $DATA_DIR"
    fi
    
    # 配置目录可能是只读挂载，忽略错误
    if [ -d "$CONFIG_DIR" ]; then
        chown -R appuser:appuser "$CONFIG_DIR" 2>/dev/null || true
    fi
    
    echo "Switching to appuser..."
    exec su-exec appuser "$@"
else
    exec "$@"
fi