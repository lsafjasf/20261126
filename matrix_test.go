package fourch

import (
	"errors"
	"math"
	"testing"
)

func TestGeneratedSquareMatricesHaveRankAndInverse(t *testing.T) {
	for size := 1; size <= 6; size++ {
		matrix, err := NewMatrix(size, size)
		if err != nil {
			t.Fatal(err)
		}
		for row := 0; row < size; row++ {
			for col := 0; col < size; col++ {
				if row == col {
					matrix[row][col] = float64(size + 2)
				} else {
					matrix[row][col] = float64((row*3+col*5)%7-3) / 10
				}
			}
		}
		rank, err := matrix.Rank(0)
		if err != nil {
			t.Fatalf("size %d rank: %v", size, err)
		}
		if rank != size {
			t.Fatalf("size %d rank = %d", size, rank)
		}
		inverse, err := matrix.Inverse(0)
		if err != nil {
			t.Fatalf("size %d inverse: %v", size, err)
		}
		product := multiplyMatrices(t, matrix, inverse)
		for row := 0; row < size; row++ {
			for col := 0; col < size; col++ {
				want := 0.0
				if row == col {
					want = 1
				}
				if math.Abs(product[row][col]-want) > 1e-9 {
					t.Fatalf("size %d product[%d][%d] = %.15g, want %.1f",
						size, row, col, product[row][col], want)
				}
			}
		}
	}
}

func TestNullspaceContainsOnlyLostDirections(t *testing.T) {
	matrix := Matrix{
		{1, 2, 0, 0},
		{2, 4, 0, 0},
	}
	rank, err := matrix.Rank(0)
	if err != nil {
		t.Fatal(err)
	}
	if rank != 1 {
		t.Fatalf("rank = %d, want 1", rank)
	}
	basis, err := matrix.Nullspace(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(basis) != 3 {
		t.Fatalf("nullspace dimension = %d, want 3", len(basis))
	}
	for i, vector := range basis {
		if len(vector) != 4 {
			t.Fatalf("basis %d length = %d", i, len(vector))
		}
		image, err := matrix.Apply(vector)
		if err != nil {
			t.Fatal(err)
		}
		for j, value := range image {
			if math.Abs(value) > 1e-12 {
				t.Fatalf("basis %d image[%d] = %g, want 0", i, j, value)
			}
		}
	}
}

func TestSingularAndInvalidMatricesReturnErrors(t *testing.T) {
	singular := Matrix{{1, 2}, {2, 4}}
	if _, err := singular.Inverse(0); !errors.Is(err, ErrSingular) {
		t.Fatalf("Inverse error = %v, want ErrSingular", err)
	}
	if _, err := (Matrix{{1, 2, 3}, {4, 5, 6}}).Inverse(0); err == nil {
		t.Fatal("expected an error when inverting a rectangular matrix")
	}
	if _, _, err := (Matrix{{1, 2}, {3}}).Dimensions(); err == nil {
		t.Fatal("expected an error for a ragged matrix")
	}
}

func multiplyMatrices(t *testing.T, a, b Matrix) Matrix {
	t.Helper()
	rowsA, colsA, err := a.Dimensions()
	if err != nil {
		t.Fatal(err)
	}
	rowsB, colsB, err := b.Dimensions()
	if err != nil {
		t.Fatal(err)
	}
	if colsA != rowsB {
		t.Fatalf("matrix dimensions %dx%d and %dx%d do not multiply", rowsA, colsA, rowsB, colsB)
	}
	product, _ := NewMatrix(rowsA, colsB)
	for row := 0; row < rowsA; row++ {
		for col := 0; col < colsB; col++ {
			for k := 0; k < colsA; k++ {
				product[row][col] += a[row][k] * b[k][col]
			}
		}
	}
	return product
}
