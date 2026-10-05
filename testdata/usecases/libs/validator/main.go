// Command validator exercises a popular pure-Go library; TestUseCaseLibraries
// compares its output under goesm with native Go.
package main

import (
	"fmt"

	"github.com/go-playground/validator/v10"
)

type Address struct {
	City string `validate:"required"`
	Zip  string `validate:"required,len=5,numeric"`
}

type User struct {
	Name      string     `validate:"required,min=2,max=10"`
	Email     string     `validate:"required,email"`
	Age       int        `validate:"gte=0,lte=130"`
	Role      string     `validate:"oneof=admin user"`
	URL       string     `validate:"omitempty,url"`
	Addresses []*Address `validate:"required,dive"`
	Password  string     `validate:"required"`
	Confirm   string     `validate:"eqfield=Password"`
	UUID      string     `validate:"omitempty,uuid4"`
}

func main() {
	v := validator.New(validator.WithRequiredStructEnabled())
	good := User{Name: "Gopher", Email: "g@go.dev", Age: 13, Role: "admin", URL: "https://go.dev", Addresses: []*Address{{City: "X", Zip: "12345"}}, Password: "p", Confirm: "p", UUID: "f47ac10b-58cc-4372-a567-0e02b2c3d479"}
	fmt.Println("good:", v.Struct(good))
	bad := User{Name: "G", Email: "nope", Age: 200, Role: "root", URL: "::", Addresses: []*Address{{Zip: "12a"}}, Password: "p", Confirm: "q", UUID: "x"}
	err := v.Struct(bad)
	for _, fe := range err.(validator.ValidationErrors) {
		fmt.Println(fe.Namespace(), fe.Tag(), fe.Param(), fe.Value())
	}
	fmt.Println(v.Var("abc@def.gh", "email"), v.Var(5, "min=10"))
}
