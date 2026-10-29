package config

import "github.com/spf13/viper"

// bindEnvironment 显式登记模块字段、默认值及环境优先级。
func bindEnvironment(v *viper.Viper) {
	v.SetDefault("app.name", "console")
	v.SetDefault("app.host", "0.0.0.0")
	v.SetDefault("app.port", 8080)
	v.SetDefault("iam.base_url", "https://127.0.0.1:9081")
	v.SetDefault("iam.token", "")
	v.SetDefault("iam.ca_file", "certs/iam.crt")
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.stdout", true)
	v.SetDefault("logging.file", "logs/console.log")
	v.SetDefault("logging.max_size_mb", 50)
	v.SetDefault("logging.max_backups", 5)
	v.SetDefault("logging.max_age_days", 14)
	v.SetDefault("logging.compress", true)

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
