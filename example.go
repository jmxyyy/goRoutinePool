// created by jmxyyy

package main

import (
	"fmt"
	"io"
	"net/http"
	"runtime"
)

func main() {
	numCPUs := runtime.NumCPU()
	pool := NewFunc(numCPUs, func(payload interface{}) interface{} {
		var result []byte
		println("hello")
		for i := 0; i < 10000; i++ {
			result = append(result, byte(i))
			if i%2000 == 0 {
				fmt.Printf("i: %d\n", i)
			}
		}

		// TODO

		return result
	})
	defer pool.Close()
	http.HandleFunc("/work", func(w http.ResponseWriter, r *http.Request) {
		input, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Internal error", http.StatusInternalServerError)
		}
		defer r.Body.Close()

		result := pool.Process(input)

		w.Write(result.([]byte))
	})
	http.ListenAndServe(":8080", nil)
}
