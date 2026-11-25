package fourch_test

import (
	"fmt"

	"fourch"
)

func ExampleInterpret() {
	observations := []fourch.Observation{
		{AzimuthDeg: 0, Channels: [4]float64{1, 0, 0, 1}},
		{AzimuthDeg: 90, Channels: [4]float64{1, 1, 0, 0}},
	}
	result, err := fourch.Interpret(
		observations,
		fourch.DefaultCandidates(),
		fourch.InterpretOptions{},
	)
	if err != nil {
		panic(err)
	}
	fmt.Println(result.Best.Kind)
	// Output: ambisonic-acn-sn3d
}
