package config

// DefaultConfigYAML 是人工维护的默认配置模板。主配置改用 YAML 是为了让设备侧
// 运维可以直接阅读和编辑；credentials 仍保持 JSON，避免把 secret 配置扩大化。
const DefaultConfigYAML = `# mgate-agent 示例配置
#
# 这个文件用于描述设备如何连接 mgate-cloud，以及如何安全调用本地 mgate.sh。
# 默认路径建议为：/etc/mgate-agent/agent.yaml

cloud:
  # mgate-cloud 的公网访问地址，必须使用 https
  base_url: "https://mgate.example.com"

  # WebSocket 主通道路径。agent 会主动连接该地址接收远程 action
  ws_path: "/api/agent/v1/ws"

  # HTTPS Pull 兜底通道路径。WebSocket 不可用时使用
  pull_path: "/api/agent/v1/pull"

  # 命令执行结果回传路径
  result_path: "/api/agent/v1/result"

  # 设备状态上报路径
  status_path: "/api/agent/v1/status"

  # 单次 HTTP 请求超时时间，单位：秒
  request_timeout_sec: 15

  # Pull 兜底轮询间隔，单位：秒
  pull_interval_sec: 10

  # 是否启用 WebSocket 主通道
  ws_enabled: true

  # 是否启用 HTTPS Pull 兜底通道
  pull_enabled: true

agent:
  # 设备显示名称，用于云端识别，建议使用易读名称
  device_name: "ufi-001"

  # 本地 mgate.sh 的绝对路径。所有本地操作都必须通过它完成
  mgate_path: "/usr/local/bin/mgate.sh"

  # agent 工作目录，用于保存运行状态、outbox 等本地数据
  work_dir: "/var/lib/mgate-agent"

  # 心跳上报间隔，单位：秒
  heartbeat_interval_sec: 30

  # 状态上报间隔，单位：秒
  status_interval_sec: 120

  # 普通命令默认超时时间，单位：秒
  default_command_timeout_sec: 30

  # 长操作命令默认超时时间，单位：秒
  long_command_timeout_sec: 180

  # 最大并发任务数。v0.1.0 建议保持为 1，避免设备侧资源竞争
  max_parallel_jobs: 1

  # 单次命令 stdout + stderr 最大回传字节数，超出后会截断
  max_output_bytes: 32768

security:
  # 设备凭证文件路径。注意：凭证文件仍为 JSON，并且权限必须是 0600
  credentials_file: "/var/lib/mgate-agent/credentials.json"

  # 本机允许执行的 action。这里是本地二次收窄，不代表云端可以动态定义 action
  allow_actions:
    - "status.snapshot"
    - "gateway.status"
    - "gateway.start"
    - "gateway.stop"
    - "wlan.scan"
    - "wlan.switch.safe"

  # 允许的云端时间偏差，单位：秒。后续签名校验会使用
  clock_skew_sec: 300

  # 是否启用严格白名单。生产环境必须保持 true
  strict_whitelist: true

logging:
  # 日志级别：debug、info、warn、error
  level: "info"

  # 审计日志路径。命令生命周期会以 JSONL 格式写入该文件
  audit_file: "/var/log/mgate-agent/audit.jsonl"
`
