package dockerdrv

import "time"

type Engine = engine

var ErrAddressUnreadable = errAddressUnreadable

func WrapEngine(d *Driver, wrap func(Engine) Engine) { d.cli = wrap(d.cli) }

func SetAfterExitReadTimeout(d *Driver, timeout time.Duration) { d.afterExitFor = timeout }
