package daemon

import "log"

type logger interface {
	Infof(format string, v ...interface{})
	Warnf(format string, v ...interface{})
	Errorf(format string, v ...interface{})
}

type defaultLoggerType struct{}

func (l defaultLoggerType) Infof(format string, v ...interface{}) {
	// omitted
}

func (l defaultLoggerType) Warnf(format string, v ...interface{}) {
	log.Printf("[daemon][warn] "+format, v...)
}

func (l defaultLoggerType) Errorf(format string, v ...interface{}) {
	log.Printf("[daemon][error] "+format, v...)
}
