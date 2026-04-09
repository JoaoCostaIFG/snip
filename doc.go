// Package snip filters CLI output through declarative YAML pipelines.
//
// Embed in another program: call [Run] with an argv-shaped []string (like [os.Args]);
// use the returned int with [os.Exit], same as the standalone binary.
//
// The snip command is in cmd/snip.
package snip
