package config

import "github.com/spf13/viper"

// bindEnvironment 显式登记模块字段及环境优先级；默认值仅在 config.yaml 定义。
func bindEnvironment(v *viper.Viper) {

	_ = v.BindEnv("app.name", "SPX_CONSOLE_NAME")
	_ = v.BindEnv("app.host", "SPX_CONSOLE_HOST")
	_ = v.BindEnv("app.port", "SPX_CONSOLE_PORT")
	_ = v.BindEnv("iam.base_url", "SPX_CONSOLE_IAM_BASE_URL")
	_ = v.BindEnv("iam.token", "SPX_CONSOLE_IAM_TOKEN")
	_ = v.BindEnv("iam.ca_file", "SPX_CONSOLE_IAM_CA_FILE")
	_ = v.BindEnv("logging.level", "SPX_CONSOLE_LOG_LEVEL")
	_ = v.BindEnv("logging.stdout", "SPX_CONSOLE_LOG_STDOUT")
	_ = v.BindEnv("logging.file", "SPX_CONSOLE_LOG_FILE")
	_ = v.BindEnv("logging.max_size_mb", "SPX_CONSOLE_LOG_MAX_SIZE_MB")
	_ = v.BindEnv("logging.max_backups", "SPX_CONSOLE_LOG_MAX_BACKUPS")
	_ = v.BindEnv("logging.max_age_days", "SPX_CONSOLE_LOG_MAX_AGE_DAYS")
	_ = v.BindEnv("logging.compress", "SPX_CONSOLE_LOG_COMPRESS")
}
