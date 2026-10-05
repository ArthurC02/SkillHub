package dockerdrv

type Engine = engine

var ErrAddressUnreadable = errAddressUnreadable

func WrapEngine(d *Driver, wrap func(Engine) Engine) { d.cli = wrap(d.cli) }
