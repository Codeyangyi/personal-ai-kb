# 日志系统使用说明

## 功能特性

- ✅ **文件日志**：所有日志自动写入文件
- ✅ **日志级别**：支持 DEBUG、INFO、WARN、ERROR 四个级别
- ✅ **日志轮转**：按日期自动轮转日志文件
- ✅ **错误分离**：错误日志单独存储在 `error-YYYY-MM-DD.log` 文件中
- ✅ **控制台输出**：可配置同时输出到控制台
- ✅ **线程安全**：支持并发写入

## 日志文件位置

日志文件默认存储在 `./logs` 目录下：

- `app-YYYY-MM-DD.log` - 所有日志（INFO、WARN、DEBUG）
- `error-YYYY-MM-DD.log` - 错误日志（ERROR）

## 配置方式

### 环境变量配置

```bash
# 日志目录（默认: ./logs）
export LOG_DIR=./logs

# 日志级别（默认: info）
# 可选值: debug, info, warn, error
export LOG_LEVEL=info

# 是否输出到控制台（默认: true）
export LOG_CONSOLE=true
```

### 代码中使用

```go
import "github.com/Codeyangyi/personal-ai-kb/logger"

// 记录不同级别的日志
logger.Debug("调试信息: %s", debugInfo)
logger.Info("普通信息: %s", info)
logger.Warn("警告信息: %s", warning)
logger.Error("错误信息: %s", error)

// 兼容标准 log 包
logger.Printf("格式化日志: %s", value)
logger.Println("普通日志")
logger.Fatalf("致命错误: %v", err)
```

## 日志格式

日志格式示例：

```
[2024-01-15 10:30:45.123] [INFO] 使用通义千问模型: qwen3-max
[2024-01-15 10:30:45.456] [ERROR] 创建向量存储失败: connection refused
```

每条日志包含：
- 时间戳（精确到毫秒）
- 日志级别
- 日志内容

## 注意事项

1. 日志文件会自动按日期轮转，每天生成新的日志文件
2. 错误日志（ERROR级别）会同时写入 `app-*.log` 和 `error-*.log`
3. 程序退出时会自动关闭日志文件
4. 如果日志目录不存在，会自动创建

