package clean

import "log/slog"

func println(string) {}

func f() {
	slog.Info("x")
	println("a local function of the same name is not the builtin")
}
