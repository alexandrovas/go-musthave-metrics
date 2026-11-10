package main

import "os"

func main() {
	os.Exit(1) // want "direct call to os.Exit in main is forbidden"
}

// os.Exit вне функции main в том же пакете main — допустим, анализатор его не
// отмечает.
func helper() {
	os.Exit(0)
}
