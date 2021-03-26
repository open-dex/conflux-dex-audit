package log

import "github.com/sirupsen/logrus"

func init() {
	logrus.SetFormatter(&logrus.TextFormatter{
		ForceColors:   true,
		FullTimestamp: true,
	})
}

// Debug logs a message at level Debug on the standard logger.
func Debug(format string, args ...interface{}) {
	logrus.Debugf(format, args...)
}

// Info logs a message at level Info on the standard logger.
func Info(format string, args ...interface{}) {
	logrus.Infof(format, args...)
}

// Warn logs a message at level Warn on the standard logger.
func Warn(format string, args ...interface{}) {
	logrus.Warnf(format, args...)
}

// Error logs a message at level Error on the standard logger.
func Error(format string, args ...interface{}) {
	logrus.Errorf(format, args...)
}

// Fatal logs a message at level Fatal on the standard logger then the process will exit with status set to non-zero.
func Fatal(format string, args ...interface{}) {
	logrus.Fatalf(format, args...)
}

// Panic logs a message at level Panic on the standard logger.
func Panic(format string, args ...interface{}) {
	logrus.Panicf(format, args...)
}
