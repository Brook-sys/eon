// Command runtime starts the autonomous epistemic runtime.
//
// The executable remains intentionally inert until the kernel vertical slice is
// assembled. Keeping the entry point buildable prevents domain code from being
// coupled to an adapter or framework during bootstrap.
package main

func main() {}
