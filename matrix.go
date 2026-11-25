package fourch

import (
	"errors"
	"fmt"
	"math"
)

// Matrix is a dense row-major matrix. It is intentionally small: the mappings
// in this package have four input channels and a handful of loudspeakers.
type Matrix [][]float64

// ErrSingular is returned when an inverse is requested for a rank-deficient or
// non-square matrix.
var ErrSingular = errors.New("matrix is singular")

// NewMatrix allocates a rows by cols zero matrix.
func NewMatrix(rows, cols int) (Matrix, error) {
	if rows <= 0 || cols <= 0 {
		return nil, fmt.Errorf("matrix dimensions must be positive")
	}
	m := make(Matrix, rows)
	for i := range m {
		m[i] = make([]float64, cols)
	}
	return m, nil
}

// Dimensions validates m and returns its row and column counts.
func (m Matrix) Dimensions() (int, int, error) {
	if len(m) == 0 {
		return 0, 0, errors.New("matrix has no rows")
	}
	cols := len(m[0])
	if cols == 0 {
		return 0, 0, errors.New("matrix has no columns")
	}
	for i, row := range m {
		if len(row) != cols {
			return 0, 0, fmt.Errorf("row %d has %d columns, want %d", i, len(row), cols)
		}
		for j, value := range row {
			if !isFinite(value) {
				return 0, 0, fmt.Errorf("matrix[%d][%d] is not finite", i, j)
			}
		}
	}
	return len(m), cols, nil
}

// Clone returns an independent copy of m.
func (m Matrix) Clone() Matrix {
	out := make(Matrix, len(m))
	for i, row := range m {
		out[i] = append([]float64(nil), row...)
	}
	return out
}

// Apply multiplies m by the column vector v.
func (m Matrix) Apply(v []float64) ([]float64, error) {
	rows, cols, err := m.Dimensions()
	if err != nil {
		return nil, err
	}
	if len(v) != cols {
		return nil, fmt.Errorf("vector length %d, want %d", len(v), cols)
	}
	for _, value := range v {
		if !isFinite(value) {
			return nil, errors.New("vector contains a non-finite value")
		}
	}
	out := make([]float64, rows)
	for i, row := range m {
		for j, value := range row {
			out[i] += value * v[j]
		}
	}
	return out, nil
}

// Rank returns the numerical rank using partial-pivoting elimination. A
// non-positive tolerance selects a scale-aware default.
func (m Matrix) Rank(tolerance float64) (int, error) {
	_, pivots, err := rref(m, tolerance)
	if err != nil {
		return 0, err
	}
	return len(pivots), nil
}

// Inverse returns the inverse of a square, full-rank matrix.
func (m Matrix) Inverse(tolerance float64) (Matrix, error) {
	rows, cols, err := m.Dimensions()
	if err != nil {
		return nil, err
	}
	if rows != cols {
		return nil, fmt.Errorf("cannot invert a %dx%d matrix", rows, cols)
	}
	eps := matrixEpsilon(m, tolerance)
	a := m.Clone()
	inv, _ := NewMatrix(rows, cols)
	for i := range inv {
		inv[i][i] = 1
	}
	for col := 0; col < cols; col++ {
		pivot := col
		for row := col + 1; row < rows; row++ {
			if math.Abs(a[row][col]) > math.Abs(a[pivot][col]) {
				pivot = row
			}
		}
		if math.Abs(a[pivot][col]) <= eps {
			return nil, ErrSingular
		}
		if pivot != col {
			a[col], a[pivot] = a[pivot], a[col]
			inv[col], inv[pivot] = inv[pivot], inv[col]
		}
		pivotValue := a[col][col]
		for j := 0; j < cols; j++ {
			a[col][j] /= pivotValue
			inv[col][j] /= pivotValue
		}
		for row := 0; row < rows; row++ {
			if row == col {
				continue
			}
			factor := a[row][col]
			if factor == 0 {
				continue
			}
			for j := 0; j < cols; j++ {
				a[row][j] -= factor * a[col][j]
				inv[row][j] -= factor * inv[col][j]
			}
		}
	}
	return inv, nil
}

// Nullspace returns a basis for the right nullspace of m. Each row of the
// result is one basis vector. The basis is derived from reduced row echelon
// form and is useful as a concrete witness of information that a mapping loses.
func (m Matrix) Nullspace(tolerance float64) (Matrix, error) {
	reduced, pivots, err := rref(m, tolerance)
	if err != nil {
		return nil, err
	}
	rows, cols, _ := m.Dimensions()
	isPivot := make([]bool, cols)
	for _, col := range pivots {
		isPivot[col] = true
	}
	out := make(Matrix, 0, cols-len(pivots))
	for free := 0; free < cols; free++ {
		if isPivot[free] {
			continue
		}
		basis := make([]float64, cols)
		basis[free] = 1
		for row, pivotCol := range pivots {
			basis[pivotCol] = -reduced[row][free]
		}
		out = append(out, basis)
	}
	if len(out) == 0 && rows > 0 {
		return Matrix{}, nil
	}
	return out, nil
}

func rref(m Matrix, tolerance float64) (Matrix, []int, error) {
	if _, _, err := m.Dimensions(); err != nil {
		return nil, nil, err
	}
	rows, cols, _ := m.Dimensions()
	eps := matrixEpsilon(m, tolerance)
	a := m.Clone()
	pivots := make([]int, 0, min(rows, cols))
	row := 0
	for col := 0; col < cols && row < rows; col++ {
		pivot := row
		for candidate := row + 1; candidate < rows; candidate++ {
			if math.Abs(a[candidate][col]) > math.Abs(a[pivot][col]) {
				pivot = candidate
			}
		}
		if math.Abs(a[pivot][col]) <= eps {
			continue
		}
		if pivot != row {
			a[row], a[pivot] = a[pivot], a[row]
		}
		pivotValue := a[row][col]
		for j := col; j < cols; j++ {
			a[row][j] /= pivotValue
		}
		for other := 0; other < rows; other++ {
			if other == row {
				continue
			}
			factor := a[other][col]
			if math.Abs(factor) <= eps {
				continue
			}
			for j := col; j < cols; j++ {
				a[other][j] -= factor * a[row][j]
			}
		}
		pivots = append(pivots, col)
		row++
	}
	return a, pivots, nil
}

func matrixEpsilon(m Matrix, tolerance float64) float64 {
	if tolerance <= 0 {
		tolerance = 1e-10
	}
	maxAbs := 0.0
	for _, row := range m {
		for _, value := range row {
			if abs := math.Abs(value); abs > maxAbs {
				maxAbs = abs
			}
		}
	}
	eps := tolerance * maxAbs
	if eps < 1e-15 {
		eps = 1e-15
	}
	return eps
}
