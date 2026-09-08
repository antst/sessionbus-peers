# Product wrappers

This split-ready tree contains product-named launchers, resident wrappers,
plugins, tests, and packaging. Wrapper code imports the bus only through
`github.com/antst/sessionbus/bus/sdk/go`; it never imports daemon internals.
Repository-local wrapper imports use
`github.com/antst/sessionbus-peers/wrappers/...`.
