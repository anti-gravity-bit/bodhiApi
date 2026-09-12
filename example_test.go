package bodhiApi_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	bodhiApi "github.com/anti-gravity-bit/bodhiApi"
)

func ExampleNew() {
	application := bodhiApi.New(bodhiApi.Info{Title: "Users API", Version: "0.0.0"})
	application.GET("/users/:id", func(requestContext bodhiApi.Context) error {
		return requestContext.JSON(http.StatusOK, map[string]string{
			"id": requestContext.Param("id"),
		})
	})

	responseRecorder := httptest.NewRecorder()
	application.Handler().ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, "/users/42", nil))
	fmt.Println(responseRecorder.Code)
	fmt.Println(responseRecorder.Body.String())
	// Output:
	// 200
	// {"id":"42"}
}

func ExampleNotFound() {
	httpError := bodhiApi.NotFound("user not found")
	fmt.Println(httpError.Status)
	fmt.Println(httpError.Code)
	fmt.Println(httpError.Message)
	// Output:
	// 404
	// not_found
	// user not found
}
