package logs

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var sugar *zap.SugaredLogger

func init() {
	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	log, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	sugar = log.Sugar()
}

func Info(args ...interface{}) {
	sugar.Info(args...)
}

func Debug(args ...interface{}) {
	sugar.Debug(args...)
}

func Error(args ...interface{}) {
	sugar.Error(args...)
}
