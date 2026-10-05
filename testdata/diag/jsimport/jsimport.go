package jsimport

//goesm:import "./missing.ts" f
func f() int

//goesm:import "./jsimport.go" g
func g(c chan int)

//goesm:import missing quotes
func h()

//goesm:import "./jsimport.go" k
func k() {}

//goesm:import "./jsimport.go" V
var V complex128

func Use() { f(); g(nil); h(); k() }
