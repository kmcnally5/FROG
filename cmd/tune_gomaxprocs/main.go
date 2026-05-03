package main

import (
	"fmt"
	"klex/eval"
	"klex/lexer"
	"klex/parser"
	"os"
	"runtime"
	"time"
)

const Version = "v0.3.6"

// Simple aggregation workload (10M elements sum).
const testProgram = `
import "parallel.lex" as p
import "datetime.lex" as dt

fn elapsed(startNanos) {
    let endNanos = dt.nowNanos()
    let elapsedNanos = endNanos - startNanos
    return elapsedNanos / 1000000000.0
}

fn main() {
    let n = 10000000
    let arr = makeArray(n, 0)
    for i in range(n) {
        arr[i] = (i * 73 + 17) % 1000000
    }

    let start = dt.nowNanos()

    sum, err = p.parallel_reduce(
        arr,
        fn(acc, x) { acc + x },
        fn(a, b) { a + b },
        8,
        0
    )

    let t = elapsed(start)
    let throughput = n / t

    if err != null {
        println("ERROR:", err)
    } else {
        println("Time: " + str(t) + " sec")
        println("Throughput: " + str(throughput) + " items/sec")
    }
}

main()
`

func runTest(gomaxprocs int) (float64, float64) {
	runtime.GOMAXPROCS(gomaxprocs)

	l := lexer.New(testProgram)
	p := parser.New(l)
	program := p.ParseProgram()

	if len(program.Errors) > 0 {
		fmt.Fprintf(os.Stderr, "Parse error\n")
		return 0, 0
	}

	eval.KLexVersion = Version
	env := eval.NewEnv()

	start := time.Now()
	result := eval.Eval(program, env)
	elapsed := time.Since(start).Seconds()

	if eval.IsError(result) {
		return 0, elapsed
	}

	return elapsed, elapsed
}

func main() {
	os.Setenv("KLEX_PATH", "./stdlib")

	gomaxprocValues := []int{1, 2, 4, 6, 8, 12, 16}

	fmt.Println("============================================================")
	fmt.Println("GOMAXPROCS TUNING BENCHMARK")
	fmt.Println("============================================================")
	fmt.Println("Testing aggregation workload (10M items sum)")
	fmt.Println("")
	fmt.Println("GOMAXPROCS | Time (sec) | Throughput (items/sec) | Efficiency")
	fmt.Println("-----------|------------|----------------------|------------")

	var bestTime float64 = 999999
	var bestGomaxprocs int = 1
	baselineTime := 0.0

	for i, gomaxprocs := range gomaxprocValues {
		_, elapsed := runTest(gomaxprocs)

		if elapsed > 0 {
			throughput := 10000000 / elapsed
			efficiency := 100.0
			if i == 0 {
				baselineTime = elapsed
				efficiency = 100.0
			} else if baselineTime > 0 {
				efficiency = (baselineTime / elapsed) * 100.0
			}

			fmt.Printf("%3d        | %9.4f  | %20.0f | %6.1f%%\n",
				gomaxprocs, elapsed, throughput, efficiency)

			if elapsed < bestTime {
				bestTime = elapsed
				bestGomaxprocs = gomaxprocs
			}
		}
	}

	fmt.Println("")
	fmt.Println("============================================================")
	fmt.Printf("WINNER: GOMAXPROCS=%d (%.4fs, %.0f items/sec)\n",
		bestGomaxprocs, bestTime, 10000000/bestTime)
	fmt.Println("============================================================")
	fmt.Println("")
	fmt.Printf("Recommendation: Set GOMAXPROCS=%d\n", bestGomaxprocs)
	fmt.Println("Usage: GOMAXPROCS=" + fmt.Sprintf("%d", bestGomaxprocs) + " ./klex <file.lex>")
	fmt.Println("")
}
