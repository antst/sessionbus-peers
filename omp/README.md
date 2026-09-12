# Sessionbus OMP peer

`omp-peer` integrates the pinned Oh My Pi command with Sessionbus. Install Oh
My Pi 18.1.17 and Bun 1.3.14 or newer first, then run this archive's `install`
script.

Interactive terminal launches retain native OMP arguments and add one managed,
per-process extension. Wrapper-owned `-g`/`--group` values select Sessionbus
groups and `-n`/`--peer-name` supplies an initial name; the live native session
ID, title, and working directory remain authoritative. Native maintenance,
help, version, print/export commands, and non-terminal stdin or stdout run
directly without managed ownership.

Daemon-owned lanes are selected by the Sessionbus launch token and accept no
command-line arguments. The lane exposes the standard Sessionbus tool from the
real native OMP session through the same per-launch extension. Installation
does not register a global OMP extension, create a native session, or make a
model request.
