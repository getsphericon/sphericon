package bad

func f() {
	println("x") // want `^rule 0: the builtin println .*; fix: log through log/slog$`
	print("x")   // want `^rule 0: the builtin print .*; fix: log through log/slog$`
}
