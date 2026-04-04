package main

import "fmt"

func main() {
	token, err := initCentralSession(".env")
	if err != nil {
		fmt.Println("Central session error:", err)
		return
	}

	fmt.Println("Central session successfully created")
	fmt.Println("Token received :", token != "")

	err = testCentralProjects(".env", token)
	if err != nil {
		fmt.Println("Central API error projects:", err)
		return
	}
}