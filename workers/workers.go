package workers

import (
	"log"
	"runtime"
)

var MaxWorkers = runtime.GOMAXPROCS(0) - 1

type ErrorResolver func(error)

func LogOnError(err error) {
	log.Println(err)
}

func PanicOnError(err error) {
	panic(err)
}
