package log

import (
	"github.com/bartdeboer/go-core"
)

// Usage:
//
//   import corelog "github.com/bartdeboer/go-core/log"
//
//   corelog.Printf("hello %s", name)
//   corelog.Debugf("details: %#v", v)

func Debug(v ...any)            { core.Log().Debug(v...) }
func Debugf(f string, a ...any) { core.Log().Debugf(f, a...) }
func Info(v ...any)             { core.Log().Info(v...) }
func Infof(f string, a ...any)  { core.Log().Infof(f, a...) }
func Warn(v ...any)             { core.Log().Warn(v...) }
func Warnf(f string, a ...any)  { core.Log().Warnf(f, a...) }
func Error(v ...any)            { core.Log().Error(v...) }
func Errorf(f string, a ...any) { core.Log().Errorf(f, a...) }
func Print(a ...any)            { Info(a...) }
func Printf(f string, a ...any) { Infof(f, a...) }
