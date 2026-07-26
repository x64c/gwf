# gw Framework

## Design Philosophy
- **Compile-time over runtime** — prefer typed structs, generics, and compile-time checks over runtime type assertions, switches, or reflection when possible.


# Debug Code vs Non-Debug Code
`//go:build debug && verbose` vs `//go:build !(debug && verbose)`
