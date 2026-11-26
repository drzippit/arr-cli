# Go, for single-binary distribution

arr-cli is written in Go. It is built first for one personal homelab but may be published later, so it matters most that it ships as a single static binary that is easy to cross-compile and install with no runtime. We considered TypeScript (fast to write, but it needs Node or Bun or a bundled runtime), Rust (also a single binary, but slower to iterate on) and Python (awkward to package).
