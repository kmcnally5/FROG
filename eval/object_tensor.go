package eval

// object_tensor.go — *Tensor is kLex's NumPy-equivalent dense
// n-dimensional array (FrogPy v1, 2026-05-23).
//
// Design choices that matter
//
//   - One storage slice per dtype (F32, F64, I64). Two of the three
//     are nil at any time depending on DType. ~72 bytes of header
//     overhead per Tensor; negligible against typical tensor
//     payloads (kilobytes to megabytes).
//   - Row-major contiguous by default. Strides == nil means
//     contiguous; views set explicit strides.
//   - Shape + Strides are immutable post-construction. reshape
//     either returns a view (shared backing slice) or a fresh
//     allocation if the layout isn't view-compatible.
//   - The compute kernels live in eval/native/tensor_kernels.c and
//     are called via cgo from tensor_compute_cgo.go (darwin / linux
//     only in v1). Windows users get a clean "tensor ops not
//     available" runtime error via the stub file.
//   - *Tensor is passed by reference in kLex (consistent with
//     *Array, *Hash). Operations like add(a, b) return a NEW
//     tensor; no in-place forms in v1.
//
// Not in v1
//
//   - Broadcasting beyond same-shape
//   - Boolean indexing
//   - Autograd
//   - dtype promotion (mixed-dtype ops error at the Go boundary
//     with a clear message; users explicitly convert)
//   - Strided views are storage-level reachable but not yet
//     reshape-producing — v1 reshape only handles contiguous → flat
//     or back.

import (
	"fmt"
	"strings"
)

// DType is the element type of a Tensor.
type DType uint8

const (
	DTypeFloat32 DType = iota
	DTypeFloat64
	DTypeInt64
)

func (d DType) String() string {
	switch d {
	case DTypeFloat32:
		return "f32"
	case DTypeFloat64:
		return "f64"
	case DTypeInt64:
		return "i64"
	}
	return fmt.Sprintf("dtype?%d", d)
}

// ElemSize returns the bytes occupied by one element of this dtype.
// Used by the C kernel callers when they need to compute buffer
// extents in bytes.
func (d DType) ElemSize() int {
	switch d {
	case DTypeFloat32:
		return 4
	case DTypeFloat64:
		return 8
	case DTypeInt64:
		return 8
	}
	return 0
}

// TensorType is the ObjectType tag for *Tensor. Distinct from
// ARRAY_OBJ so introspection works.
const TENSOR_OBJ ObjectType = "TENSOR"

// Tensor is a contiguous (in v1) n-dimensional array of one of the
// supported dtypes. Exactly one of F32/F64/I64 is non-nil; which one
// is determined by DType.
type Tensor struct {
	DType   DType
	Shape   []int
	Strides []int // nil == contiguous row-major
	F32     []float32
	F64     []float64
	I64     []int64
}

func (t *Tensor) Type() ObjectType { return TENSOR_OBJ }

// Inspect produces a NumPy-flavoured short repr: "tensor(f64, shape=[2, 3])".
// We deliberately don't print the data — for big tensors that would flood
// the console. Use t.preview() (TBD builtin) for a head-of-data dump.
func (t *Tensor) Inspect() string {
	var buf strings.Builder
	buf.WriteString("tensor(")
	buf.WriteString(t.DType.String())
	buf.WriteString(", shape=[")
	for i, s := range t.Shape {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(fmt.Sprintf("%d", s))
	}
	buf.WriteString("])")
	return buf.String()
}

// Numel returns the total element count (product of Shape). For a
// zero-dim or empty-shape tensor that's 1 (scalar) or 0 respectively.
func (t *Tensor) Numel() int {
	if len(t.Shape) == 0 {
		return 1 // scalar
	}
	n := 1
	for _, s := range t.Shape {
		n *= s
	}
	return n
}

// IsContiguous reports whether the tensor's data layout is the
// default row-major dense layout (no view-strides). Most kernels
// require this — non-contiguous tensors need to be materialised
// first (via Clone) before being handed to a C kernel.
func (t *Tensor) IsContiguous() bool {
	return t.Strides == nil
}

// computeContiguousStrides fills Strides for a shape under
// row-major layout. Used internally by constructors that take a
// shape and need to compute strides for views. Stored only when
// the tensor is a non-default view — IsContiguous() == true means
// "use the natural row-major strides", which the kernels assume.
func computeContiguousStrides(shape []int) []int {
	if len(shape) == 0 {
		return nil
	}
	strides := make([]int, len(shape))
	s := 1
	for i := len(shape) - 1; i >= 0; i-- {
		strides[i] = s
		s *= shape[i]
	}
	return strides
}

// newTensorFromShape allocates a fresh contiguous tensor of the
// given dtype + shape. The data slice is zero-initialised by Go's
// default. Internal helper used by both kLex builtins (zeros) and
// the cgo result allocator.
func newTensorFromShape(dtype DType, shape []int) *Tensor {
	n := 1
	for _, s := range shape {
		n *= s
	}
	t := &Tensor{DType: dtype, Shape: append([]int(nil), shape...)}
	switch dtype {
	case DTypeFloat32:
		t.F32 = make([]float32, n)
	case DTypeFloat64:
		t.F64 = make([]float64, n)
	case DTypeInt64:
		t.I64 = make([]int64, n)
	}
	return t
}

// dtypeFromName converts a kLex-level dtype string ("f32", "f64",
// "i64", "float32", "float64", "int64") to a DType. Returns
// (0, false) on unknown name. Used by tensor constructor builtins
// that accept a dtype string argument.
func dtypeFromName(name string) (DType, bool) {
	switch name {
	case "f32", "float32":
		return DTypeFloat32, true
	case "f64", "float64":
		return DTypeFloat64, true
	case "i64", "int64":
		return DTypeInt64, true
	}
	return 0, false
}
