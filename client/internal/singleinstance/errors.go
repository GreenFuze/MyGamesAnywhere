package singleinstance

import "errors"

var ErrAlreadyRunning = errors.New("MGA Client agent is already running for this OS user")
