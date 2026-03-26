#!/bin/sh
set -e

# 修复挂载目录的权限问题
# 当从宿主机挂载目录时，目录可能属于 root 或其他用户
# 需要在启动前修复权限，以便 appuser 可以写入

DATA_DIR="${DATA_DIR:-/app/data}"
CONFIG_DIR="${CONFIG_DIR:-/app/config}"

# 检查是否以 root 运行
if [ "$(id -u)" = "0" ]; then
    echo "Running as root, fixing permissions..."
    
    # 修复数据目录权限
    if [ -d "$DATA_DIR" ]; then
        chown -R appuser:appuser "$DATA_DIR"
        echo "Fixed permissions for $DATA_DIR"
    fi
    
    # 修复配置目录权限
    if [ -d "$CONFIG_DIR" ]; then
        chown -R appuser:appuser "$CONFIG_DIR"
        echo "Fixed permissions for $CONFIG_DIR"
    fi
    
    # 切换到 appuser 执行
    echo "Switching to appuser..."
    exec su-exec appuser "$@"
else
    # 已经是非 root 用户，直接执行
    exec "$@"
fi