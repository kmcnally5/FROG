package eval

// builtins_tensor.go — kLex-side dispatch for the FrogPy v1 tensor
// surface. Each Go function in here is a thin shim that:
//
//   1. Validates argument count + types.
//   2. Dispatches by Tensor.DType into the matching cgo wrapper.
//   3. Returns either the new *Tensor or a kLex *Error.
//
// API style (per design discussion 2026-05-23): flat builtins with
// a `_tensor_` prefix, then stdlib/tensor.lex wraps them in
// NumPy-flavoured names (zeros, add, shape, dtype). This keeps each
// op a single Go function — no per-call method-dispatch indirection
// and a clear search path from `t.add` in user code through the
// stdlib wrapper to the C kernel.

import (
	"fmt"
	"math/rand"
	"time"

	"klex/ast"
)

func init() {
	// _tensor_zeros(shape: array, dtype: string) -> tensor
	//
	// shape is a kLex array of integers; dtype is one of
	// "f32"/"float32", "f64"/"float64", "i64"/"int64".
	// Allocates a fresh contiguous tensor with all elements set
	// to zero (Go's slice-of-numeric-type default).
	Builtins["_tensor_zeros"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_tensor_zeros expects 2 arguments (shape, dtype)", ast.Pos{})
		}
		shapeArr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_zeros: shape must be an array, got %s", args[0].Type()), ast.Pos{})
		}
		dtypeStr, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_zeros: dtype must be a string, got %s", args[1].Type()), ast.Pos{})
		}
		dt, dok := dtypeFromName(dtypeStr.Value)
		if !dok {
			return typeError(fmt.Sprintf("_tensor_zeros: unknown dtype %q (want f32/f64/i64)", dtypeStr.Value), ast.Pos{})
		}
		shape, err := tensorShapeFromArray(shapeArr)
		if err != nil {
			return err
		}
		return newTensorFromShape(dt, shape)
	}}

	// _tensor_full(shape: array, value: number, dtype: string) -> tensor
	//
	// Allocates a fresh contiguous tensor with every element set to
	// `value`. Matches NumPy's np.full. Bypasses the
	// from_array→reshape ceremony required to build a non-zero
	// constant matrix — useful for benchmark setup, weight init, and
	// constant operands.
	//
	// Value conversion rules mirror _tensor_from_array:
	//   - f32 / f64 accept Integer or Float
	//   - i64 accepts Integer only; Float is rejected without explicit
	//     conversion (consistent with kLex's strict-typing policy)
	Builtins["_tensor_full"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 3 {
			return runtimeError("_tensor_full expects 3 arguments (shape, value, dtype)", ast.Pos{})
		}
		shapeArr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_full: shape must be an array, got %s", args[0].Type()), ast.Pos{})
		}
		dtypeStr, ok := args[2].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_full: dtype must be a string, got %s", args[2].Type()), ast.Pos{})
		}
		dt, dok := dtypeFromName(dtypeStr.Value)
		if !dok {
			return typeError(fmt.Sprintf("_tensor_full: unknown dtype %q (want f32/f64/i64)", dtypeStr.Value), ast.Pos{})
		}
		shape, errShape := tensorShapeFromArray(shapeArr)
		if errShape != nil {
			return errShape
		}
		t := newTensorFromShape(dt, shape)
		switch dt {
		case DTypeFloat32:
			var v float32
			switch x := args[1].(type) {
			case *Integer:
				v = float32(x.Value)
			case *Float:
				v = float32(x.Value)
			default:
				return typeError(fmt.Sprintf("_tensor_full: value must be a number for f32 dtype, got %s", args[1].Type()), ast.Pos{})
			}
			for i := range t.F32 {
				t.F32[i] = v
			}
		case DTypeFloat64:
			var v float64
			switch x := args[1].(type) {
			case *Integer:
				v = float64(x.Value)
			case *Float:
				v = x.Value
			default:
				return typeError(fmt.Sprintf("_tensor_full: value must be a number for f64 dtype, got %s", args[1].Type()), ast.Pos{})
			}
			for i := range t.F64 {
				t.F64[i] = v
			}
		case DTypeInt64:
			var v int64
			switch x := args[1].(type) {
			case *Integer:
				v = int64(x.Value)
			case *Float:
				return typeError("_tensor_full: cannot fill i64 tensor with a float value (use an integer)", ast.Pos{})
			default:
				return typeError(fmt.Sprintf("_tensor_full: value must be a number for i64 dtype, got %s", args[1].Type()), ast.Pos{})
			}
			for i := range t.I64 {
				t.I64[i] = v
			}
		}
		return t
	}}

	// _tensor_random(shape: array, dtype: string, seed: int) -> tensor
	//
	// Allocates a fresh contiguous tensor filled with pseudo-random
	// values. Matches NumPy's np.random.rand for the float dtypes:
	// uniform on [0, 1). For i64 a separate semantic applies — the
	// full int64 range is used (consistent with NumPy's default int
	// generator).
	//
	// seed semantics:
	//   seed > 0  → deterministic. Same seed + same shape produces
	//               identical output across runs. Use this for
	//               reproducible benchmarks and tests.
	//   seed == 0 → time-seeded (non-reproducible). Use this when
	//               you want fresh entropy each run.
	//
	// The PRNG is Go's math/rand (PCG-family). Not cryptographically
	// secure — for crypto use stdlib/crypto.lex, not this.
	Builtins["_tensor_random"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 3 {
			return runtimeError("_tensor_random expects 3 arguments (shape, dtype, seed)", ast.Pos{})
		}
		shapeArr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_random: shape must be an array, got %s", args[0].Type()), ast.Pos{})
		}
		dtypeStr, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_random: dtype must be a string, got %s", args[1].Type()), ast.Pos{})
		}
		seedObj, ok := args[2].(*Integer)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_random: seed must be an integer, got %s", args[2].Type()), ast.Pos{})
		}
		dt, dok := dtypeFromName(dtypeStr.Value)
		if !dok {
			return typeError(fmt.Sprintf("_tensor_random: unknown dtype %q (want f32/f64/i64)", dtypeStr.Value), ast.Pos{})
		}
		shape, errShape := tensorShapeFromArray(shapeArr)
		if errShape != nil {
			return errShape
		}
		var seed int64
		if seedObj.Value == 0 {
			seed = time.Now().UnixNano()
		} else {
			seed = int64(seedObj.Value)
		}
		r := rand.New(rand.NewSource(seed))
		t := newTensorFromShape(dt, shape)
		switch dt {
		case DTypeFloat32:
			for i := range t.F32 {
				t.F32[i] = r.Float32()
			}
		case DTypeFloat64:
			for i := range t.F64 {
				t.F64[i] = r.Float64()
			}
		case DTypeInt64:
			for i := range t.I64 {
				t.I64[i] = r.Int63()
			}
		}
		return t
	}}

	// _tensor_from_array(data: array, dtype: string) -> tensor
	//
	// Converts a flat 1-D kLex array (of numbers) to a 1-D tensor
	// of the given dtype. Element conversion is strict: Float64
	// elements convert losslessly to f64, are truncated to f32,
	// and rejected for i64; Integer elements convert losslessly
	// to i64 and as floats for f32/f64. Mixed-type arrays where
	// some element doesn't fit the target dtype error cleanly.
	//
	// For multi-dim construction in v1, build a 1-D tensor here
	// then reshape (TBD). Keeps the v1 surface tight.
	Builtins["_tensor_from_array"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_tensor_from_array expects 2 arguments (data, dtype)", ast.Pos{})
		}
		dataArr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_from_array: data must be an array, got %s", args[0].Type()), ast.Pos{})
		}
		dtypeStr, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_from_array: dtype must be a string, got %s", args[1].Type()), ast.Pos{})
		}
		dt, dok := dtypeFromName(dtypeStr.Value)
		if !dok {
			return typeError(fmt.Sprintf("_tensor_from_array: unknown dtype %q", dtypeStr.Value), ast.Pos{})
		}
		t := newTensorFromShape(dt, []int{len(dataArr.Elements)})
		for i, el := range dataArr.Elements {
			if err := tensorStoreScalar(t, i, el); err != nil {
				return err
			}
		}
		return t
	}}

	// Element-wise binary ops. Each one is a thin wrapper around
	// elementWiseBinary with the right kernel triple plugged in.
	// All four (add/sub/mul/div) follow identical shape rules:
	//   - same dtype on both operands
	//   - same shape (no broadcasting in v1)
	//   - both contiguous
	// div additionally pre-scans the i64 divisor for zeros (the C
	// kernel doesn't guard; we surface a clean kLex error with the
	// offending index).

	// _tensor_add(a, b) → tensor
	// Element-wise a + b. Supports tensor+tensor (same dtype, broadcastable
	// shapes) and tensor+scalar / scalar+tensor mixed forms. Allocates a fresh
	// output tensor; use _tensor_add_inplace to avoid the allocation.
	Builtins["_tensor_add"] = &Builtin{Fn: func(args []Object) Object {
		return elementWiseBinary(args, "_tensor_add", binaryKernel{
			f32: tensorAddF32, f64: tensorAddF64, i64: tensorAddI64,
		})
	}}

	// _tensor_sub(a, b) → tensor
	// Element-wise a - b. Same dtype/shape rules as _tensor_add.
	Builtins["_tensor_sub"] = &Builtin{Fn: func(args []Object) Object {
		return elementWiseBinary(args, "_tensor_sub", binaryKernel{
			f32: tensorSubF32, f64: tensorSubF64, i64: tensorSubI64,
		})
	}}

	// _tensor_mul(a, b) → tensor
	// Element-wise a * b. Same dtype/shape rules as _tensor_add.
	Builtins["_tensor_mul"] = &Builtin{Fn: func(args []Object) Object {
		return elementWiseBinary(args, "_tensor_mul", binaryKernel{
			f32: tensorMulF32, f64: tensorMulF64, i64: tensorMulI64,
		})
	}}

	// ── unary element-wise ops ──
	// neg/abs supported across all three dtypes. The transcendentals
	// (exp/log/sqrt/sin/cos) are float-only; passing an i64 tensor
	// surfaces a clean "use f32 or f64" error from elementWiseUnary.

	// _tensor_neg(a) → tensor
	// Element-wise -a. Returns a fresh tensor with the same shape and dtype as a.
	// Supported for all dtypes (f32, f64, i64).
	Builtins["_tensor_neg"] = &Builtin{Fn: func(args []Object) Object {
		return elementWiseUnary(args, "_tensor_neg", unaryKernel{
			f32: tensorNegF32, f64: tensorNegF64, i64: tensorNegI64,
		})
	}}

	// _tensor_abs(a) → tensor
	// Element-wise abs(a). Returns a fresh tensor with the same shape and dtype as a.
	// Supported for all dtypes (f32, f64, i64).
	Builtins["_tensor_abs"] = &Builtin{Fn: func(args []Object) Object {
		return elementWiseUnary(args, "_tensor_abs", unaryKernel{
			f32: tensorAbsF32, f64: tensorAbsF64, i64: tensorAbsI64,
		})
	}}

	// _tensor_exp(a) → tensor
	// Element-wise exp(a). Float-only (f32/f64); passing i64 errors cleanly.
	Builtins["_tensor_exp"] = &Builtin{Fn: func(args []Object) Object {
		return elementWiseUnary(args, "_tensor_exp", unaryKernel{
			f32: tensorExpF32, f64: tensorExpF64,
		})
	}}

	// _tensor_log(a) → tensor
	// Element-wise natural log(a). Float-only (f32/f64). Negative inputs produce
	// NaN silently (IEEE 754); use safe() if you want to detect domain errors.
	Builtins["_tensor_log"] = &Builtin{Fn: func(args []Object) Object {
		return elementWiseUnary(args, "_tensor_log", unaryKernel{
			f32: tensorLogF32, f64: tensorLogF64,
		})
	}}

	// _tensor_sqrt(a) → tensor
	// Element-wise sqrt(a). Float-only (f32/f64). Negative inputs produce NaN
	// silently (IEEE 754).
	Builtins["_tensor_sqrt"] = &Builtin{Fn: func(args []Object) Object {
		return elementWiseUnary(args, "_tensor_sqrt", unaryKernel{
			f32: tensorSqrtF32, f64: tensorSqrtF64,
		})
	}}

	// _tensor_sin(a) → tensor
	// Element-wise sin(a). Float-only (f32/f64).
	Builtins["_tensor_sin"] = &Builtin{Fn: func(args []Object) Object {
		return elementWiseUnary(args, "_tensor_sin", unaryKernel{
			f32: tensorSinF32, f64: tensorSinF64,
		})
	}}

	// _tensor_cos(a) → tensor
	// Element-wise cos(a). Float-only (f32/f64).
	Builtins["_tensor_cos"] = &Builtin{Fn: func(args []Object) Object {
		return elementWiseUnary(args, "_tensor_cos", unaryKernel{
			f32: tensorCosF32, f64: tensorCosF64,
		})
	}}

	// _tensor_pow(a, b) → tensor
	// Element-wise a ** b. All dtypes. Negative i64 exponents error cleanly
	// (result would be fractional, not representable in i64 — use f32/f64).
	// Float kernels handle negative exponents natively via libm pow.
	Builtins["_tensor_pow"] = &Builtin{Fn: func(args []Object) Object {
		return elementWiseBinary(args, "_tensor_pow", binaryKernel{
			f32: tensorPowF32,
			f64: tensorPowF64,
			i64: tensorPowI64,
			// preCheck rejects negative i64 exponents — the result
			// would be |x|<1 for |a|>1, which isn't representable in
			// i64. Floats handle negative exponents natively (libm
			// pow). Matches NumPy: np.power(2, -1, dtype=np.int64)
			// raises a ValueError.
			preCheck: func(a, b *Tensor) string {
				if a.DType != DTypeInt64 {
					return ""
				}
				for i, v := range b.I64 {
					if v < 0 {
						return fmt.Sprintf("negative exponent %d at index %d not representable in i64 (use f32 or f64)", v, i)
					}
				}
				return ""
			},
		})
	}}

	// _tensor_div(a, b) → tensor
	// Element-wise a / b. All dtypes. For i64, a pre-scan detects divide-by-zero
	// and surfaces a clean kLex error with the offending index. Float dtypes
	// follow IEEE 754 (n/0 = ±Inf, 0/0 = NaN) — no error raised.
	Builtins["_tensor_div"] = &Builtin{Fn: func(args []Object) Object {
		return elementWiseBinary(args, "_tensor_div", binaryKernel{
			f32: tensorDivF32,
			f64: tensorDivF64,
			i64: tensorDivI64,
			// preCheck fires AFTER shape/dtype/contiguity validation
			// but BEFORE the kernel dispatches. div uses it to scan
			// the i64 divisor for zeros — the C i64 kernel doesn't
			// guard (an unchecked `a/0` is undefined behaviour in C),
			// so we catch it here and surface a clean kLex error
			// with the offending index. Float kernels rely on IEEE
			// 754 — n/0 = ±Inf, 0/0 = NaN — no error needed.
			preCheck: func(a, b *Tensor) string {
				if a.DType != DTypeInt64 {
					return ""
				}
				for i, v := range b.I64 {
					if v == 0 {
						return fmt.Sprintf("division by zero at index %d", i)
					}
				}
				return ""
			},
		})
	}}

	// ── in-place element-wise ops ──
	//
	// Each `_inplace` variant mutates its first argument and returns
	// it (so callers can chain). The C kernels already permit aliasing
	// (`out` may equal `a` or `b`), so we just pass a.F* as both the
	// destination and the first source — no fresh allocation, no zero-
	// initialisation. On M4 this restores the kernel's full 119 GB/s
	// throughput for f64 add (vs ~79 GB/s for the allocating variant).
	//
	// Semantics:
	//   _tensor_add_inplace(a, b)   →   a[i] = a[i] + b[i]   (returns a)
	//   _tensor_neg_inplace(a)      →   a[i] = -a[i]         (returns a)
	//
	// Shape / dtype / contiguity rules are identical to the non-inplace
	// variants — only the result buffer differs.

	// _tensor_add_inplace(a, b) → tensor
	// In-place element-wise add: a[i] = a[i] + b[i]. Returns a. Avoids allocation
	// for higher throughput than _tensor_add. Both tensors must have identical
	// dtype and shape (no broadcasting for in-place ops).
	Builtins["_tensor_add_inplace"] = &Builtin{Fn: func(args []Object) Object {
		return elementWiseBinaryInplace(args, "_tensor_add_inplace", binaryKernel{
			f32: tensorAddF32, f64: tensorAddF64, i64: tensorAddI64,
		})
	}}

	// _tensor_sub_inplace(a, b) → tensor
	// In-place element-wise subtract: a[i] = a[i] - b[i]. Returns a.
	Builtins["_tensor_sub_inplace"] = &Builtin{Fn: func(args []Object) Object {
		return elementWiseBinaryInplace(args, "_tensor_sub_inplace", binaryKernel{
			f32: tensorSubF32, f64: tensorSubF64, i64: tensorSubI64,
		})
	}}

	// _tensor_mul_inplace(a, b) → tensor
	// In-place element-wise multiply: a[i] = a[i] * b[i]. Returns a.
	Builtins["_tensor_mul_inplace"] = &Builtin{Fn: func(args []Object) Object {
		return elementWiseBinaryInplace(args, "_tensor_mul_inplace", binaryKernel{
			f32: tensorMulF32, f64: tensorMulF64, i64: tensorMulI64,
		})
	}}

	// _tensor_div_inplace(a, b) → tensor
	// In-place element-wise divide: a[i] = a[i] / b[i]. Returns a. i64 div-by-zero
	// errors cleanly; float dtypes follow IEEE 754.
	Builtins["_tensor_div_inplace"] = &Builtin{Fn: func(args []Object) Object {
		return elementWiseBinaryInplace(args, "_tensor_div_inplace", binaryKernel{
			f32: tensorDivF32,
			f64: tensorDivF64,
			i64: tensorDivI64,
			preCheck: func(a, b *Tensor) string {
				if a.DType != DTypeInt64 {
					return ""
				}
				for i, v := range b.I64 {
					if v == 0 {
						return fmt.Sprintf("division by zero at index %d", i)
					}
				}
				return ""
			},
		})
	}}

	// _tensor_pow_inplace(a, b) → tensor
	// In-place element-wise power: a[i] = a[i] ** b[i]. Returns a. Negative i64
	// exponents error cleanly.
	Builtins["_tensor_pow_inplace"] = &Builtin{Fn: func(args []Object) Object {
		return elementWiseBinaryInplace(args, "_tensor_pow_inplace", binaryKernel{
			f32: tensorPowF32,
			f64: tensorPowF64,
			i64: tensorPowI64,
			preCheck: func(a, b *Tensor) string {
				if a.DType != DTypeInt64 {
					return ""
				}
				for i, v := range b.I64 {
					if v < 0 {
						return fmt.Sprintf("negative exponent %d at index %d not representable in i64 (use f32 or f64)", v, i)
					}
				}
				return ""
			},
		})
	}}

	// _tensor_neg_inplace(a) → tensor
	// In-place element-wise negate: a[i] = -a[i]. Returns a. All dtypes.
	Builtins["_tensor_neg_inplace"] = &Builtin{Fn: func(args []Object) Object {
		return elementWiseUnaryInplace(args, "_tensor_neg_inplace", unaryKernel{
			f32: tensorNegF32, f64: tensorNegF64, i64: tensorNegI64,
		})
	}}

	// _tensor_abs_inplace(a) → tensor
	// In-place element-wise absolute value: a[i] = abs(a[i]). Returns a. All
	// dtypes. For i64, INT64_MIN wraps to itself (matches NumPy).
	Builtins["_tensor_abs_inplace"] = &Builtin{Fn: func(args []Object) Object {
		return elementWiseUnaryInplace(args, "_tensor_abs_inplace", unaryKernel{
			f32: tensorAbsF32, f64: tensorAbsF64, i64: tensorAbsI64,
		})
	}}

	// _tensor_exp_inplace(a) → tensor
	// In-place element-wise exp(a). Returns a. Float-only (f32/f64); rejects i64.
	Builtins["_tensor_exp_inplace"] = &Builtin{Fn: func(args []Object) Object {
		return elementWiseUnaryInplace(args, "_tensor_exp_inplace", unaryKernel{
			f32: tensorExpF32, f64: tensorExpF64,
		})
	}}

	// _tensor_log_inplace(a) → tensor
	// In-place element-wise log(a). Returns a. Float-only. Negative inputs produce
	// NaN silently (IEEE 754).
	Builtins["_tensor_log_inplace"] = &Builtin{Fn: func(args []Object) Object {
		return elementWiseUnaryInplace(args, "_tensor_log_inplace", unaryKernel{
			f32: tensorLogF32, f64: tensorLogF64,
		})
	}}

	// _tensor_sqrt_inplace(a) → tensor
	// In-place element-wise sqrt(a). Returns a. Float-only. Negative inputs produce
	// NaN silently (IEEE 754).
	Builtins["_tensor_sqrt_inplace"] = &Builtin{Fn: func(args []Object) Object {
		return elementWiseUnaryInplace(args, "_tensor_sqrt_inplace", unaryKernel{
			f32: tensorSqrtF32, f64: tensorSqrtF64,
		})
	}}

	// _tensor_sin_inplace(a) → tensor
	// In-place element-wise sin(a). Returns a. Float-only.
	Builtins["_tensor_sin_inplace"] = &Builtin{Fn: func(args []Object) Object {
		return elementWiseUnaryInplace(args, "_tensor_sin_inplace", unaryKernel{
			f32: tensorSinF32, f64: tensorSinF64,
		})
	}}

	// _tensor_cos_inplace(a) → tensor
	// In-place element-wise cos(a). Returns a. Float-only.
	Builtins["_tensor_cos_inplace"] = &Builtin{Fn: func(args []Object) Object {
		return elementWiseUnaryInplace(args, "_tensor_cos_inplace", unaryKernel{
			f32: tensorCosF32, f64: tensorCosF64,
		})
	}}

	// _tensor_shape(t: tensor) -> array
	// Returns a fresh kLex array of integers describing t's shape.
	Builtins["_tensor_shape"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_tensor_shape expects 1 argument", ast.Pos{})
		}
		t, ok := args[0].(*Tensor)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_shape: argument must be a tensor, got %s", args[0].Type()), ast.Pos{})
		}
		els := make([]Object, len(t.Shape))
		for i, s := range t.Shape {
			els[i] = intObj(s)
		}
		return &Array{Elements: els}
	}}

	// _tensor_dtype(t: tensor) -> string
	// Returns the dtype name ("f32", "f64", "i64").
	Builtins["_tensor_dtype"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_tensor_dtype expects 1 argument", ast.Pos{})
		}
		t, ok := args[0].(*Tensor)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_dtype: argument must be a tensor, got %s", args[0].Type()), ast.Pos{})
		}
		return &String{Value: t.DType.String()}
	}}

	// _tensor_numel(t: tensor) -> int
	// Total element count (product of shape).
	Builtins["_tensor_numel"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_tensor_numel expects 1 argument", ast.Pos{})
		}
		t, ok := args[0].(*Tensor)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_numel: argument must be a tensor, got %s", args[0].Type()), ast.Pos{})
		}
		return intObj(t.Numel())
	}}

	// _tensor_get(t: tensor, idx: int) -> number
	// Linear element access — treats the tensor as a flat 1-D
	// view of its logical data. Used by stdlib/tensor.lex's
	// preview/print helpers and by users who want a single scalar
	// out without computing the multi-index. Bounds-checked.
	//
	// Works on strided views (from _tensor_slice) too: the flat
	// index is translated through the tensor's Strides to find the
	// physical backing-slice offset.
	Builtins["_tensor_get"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_tensor_get expects 2 arguments (tensor, idx)", ast.Pos{})
		}
		t, ok := args[0].(*Tensor)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_get: first argument must be a tensor, got %s", args[0].Type()), ast.Pos{})
		}
		idx, ok := args[1].(*Integer)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_get: index must be int, got %s", args[1].Type()), ast.Pos{})
		}
		n := t.Numel()
		i := idx.Value
		if i < 0 || i >= n {
			return runtimeError(fmt.Sprintf("_tensor_get: index %d out of bounds (numel %d)", i, n), ast.Pos{})
		}
		off := i
		if !t.IsContiguous() {
			off = stridedOffsetFromFlat(i, t.Shape, t.Strides)
		}
		switch t.DType {
		case DTypeFloat32:
			return &Float{Value: float64(t.F32[off])}
		case DTypeFloat64:
			return &Float{Value: t.F64[off]}
		case DTypeInt64:
			return intObj(int(t.I64[off]))
		}
		return runtimeError("_tensor_get: unreachable dtype", ast.Pos{})
	}}

	// ── reductions ──
	//
	// Each reduction takes 1 tensor and returns a scalar. Return type
	// policy:
	//   - sum / min / max     : same-dtype scalar (Float for f32/f64,
	//                           Integer for i64)
	//   - mean                : always Float (matches NumPy convention;
	//                           computed in Go as sum / n so the
	//                           precision policy stays tied to sum)
	//   - argmin / argmax     : Integer index
	//
	// Empty-tensor policy:
	//   - sum on empty returns 0 / 0.0 (the identity element).
	//   - mean / min / max / argmin / argmax on empty error cleanly —
	//     they have no well-defined value. The Go layer catches n == 0
	//     before dispatching to the C kernel (which assumes n >= 1).
	//
	// All reductions require a contiguous tensor in v1. Non-contiguous
	// inputs surface the same "not yet supported" message used by
	// element-wise ops.

	// _tensor_sum(t) → number
	// Sum of all elements. Returns a same-dtype scalar (Float for f32/f64,
	// Integer for i64). Returns 0 for empty tensors (sum's identity element).
	Builtins["_tensor_sum"] = &Builtin{Fn: func(args []Object) Object {
		t, errObj := reductionPrologue(args, "_tensor_sum", false)
		if errObj != nil {
			return errObj
		}
		switch t.DType {
		case DTypeFloat32:
			return &Float{Value: float64(tensorSumF32(t.F32))}
		case DTypeFloat64:
			return &Float{Value: tensorSumF64(t.F64)}
		case DTypeInt64:
			return intObj(int(tensorSumI64(t.I64)))
		}
		return runtimeError("_tensor_sum: unreachable dtype", ast.Pos{})
	}}

	// _tensor_mean(t) → float
	// Mean of all elements. Always returns Float regardless of dtype (matches
	// NumPy's np.mean on integer arrays). Computed as sum/n in Go; precision
	// follows the dtype's accumulator (f32 accumulates in float, f64 in double,
	// i64 widened to float64). Errors on empty tensor.
	Builtins["_tensor_mean"] = &Builtin{Fn: func(args []Object) Object {
		t, errObj := reductionPrologue(args, "_tensor_mean", true)
		if errObj != nil {
			return errObj
		}
		n := t.Numel()
		switch t.DType {
		case DTypeFloat32:
			return &Float{Value: float64(tensorSumF32(t.F32)) / float64(n)}
		case DTypeFloat64:
			return &Float{Value: tensorSumF64(t.F64) / float64(n)}
		case DTypeInt64:
			return &Float{Value: float64(tensorSumI64(t.I64)) / float64(n)}
		}
		return runtimeError("_tensor_mean: unreachable dtype", ast.Pos{})
	}}

	// _tensor_min(t) → number
	// Minimum element. Returns a same-dtype scalar (Float for f32/f64, Integer
	// for i64). Errors on empty tensor.
	Builtins["_tensor_min"] = &Builtin{Fn: func(args []Object) Object {
		t, errObj := reductionPrologue(args, "_tensor_min", true)
		if errObj != nil {
			return errObj
		}
		switch t.DType {
		case DTypeFloat32:
			return &Float{Value: float64(tensorMinF32(t.F32))}
		case DTypeFloat64:
			return &Float{Value: tensorMinF64(t.F64)}
		case DTypeInt64:
			return intObj(int(tensorMinI64(t.I64)))
		}
		return runtimeError("_tensor_min: unreachable dtype", ast.Pos{})
	}}

	// _tensor_max(t) → number
	// Maximum element. Returns a same-dtype scalar (Float for f32/f64, Integer
	// for i64). Errors on empty tensor.
	Builtins["_tensor_max"] = &Builtin{Fn: func(args []Object) Object {
		t, errObj := reductionPrologue(args, "_tensor_max", true)
		if errObj != nil {
			return errObj
		}
		switch t.DType {
		case DTypeFloat32:
			return &Float{Value: float64(tensorMaxF32(t.F32))}
		case DTypeFloat64:
			return &Float{Value: tensorMaxF64(t.F64)}
		case DTypeInt64:
			return intObj(int(tensorMaxI64(t.I64)))
		}
		return runtimeError("_tensor_max: unreachable dtype", ast.Pos{})
	}}

	// _tensor_argmin(t) → int
	// Linear index of the minimum element (first occurrence on ties). Errors on
	// empty tensor.
	Builtins["_tensor_argmin"] = &Builtin{Fn: func(args []Object) Object {
		t, errObj := reductionPrologue(args, "_tensor_argmin", true)
		if errObj != nil {
			return errObj
		}
		switch t.DType {
		case DTypeFloat32:
			return intObj(tensorArgminF32(t.F32))
		case DTypeFloat64:
			return intObj(tensorArgminF64(t.F64))
		case DTypeInt64:
			return intObj(tensorArgminI64(t.I64))
		}
		return runtimeError("_tensor_argmin: unreachable dtype", ast.Pos{})
	}}

	// _tensor_argmax(t) → int
	// Linear index of the maximum element (first occurrence on ties). Errors on
	// empty tensor.
	Builtins["_tensor_argmax"] = &Builtin{Fn: func(args []Object) Object {
		t, errObj := reductionPrologue(args, "_tensor_argmax", true)
		if errObj != nil {
			return errObj
		}
		switch t.DType {
		case DTypeFloat32:
			return intObj(tensorArgmaxF32(t.F32))
		case DTypeFloat64:
			return intObj(tensorArgmaxF64(t.F64))
		case DTypeInt64:
			return intObj(tensorArgmaxI64(t.I64))
		}
		return runtimeError("_tensor_argmax: unreachable dtype", ast.Pos{})
	}}

	// ── axis-aware reductions (N-D, single-axis collapse) ─────
	//
	// Each *_axis builtin takes (tensor, axis) and returns a tensor
	// with the named axis removed (drop variant) or kept at size 1
	// (keepdims variant). axis may be negative; negative-indexed
	// from the end of the shape (-1 = last axis).
	//
	// Output dtype:
	//   sum / min / max → same dtype as input
	//   mean → always f64 (matches scalar mean's policy)
	//   argmin / argmax → always i64 (index into the reduced axis)
	//
	// Empty-axis policy: sum's identity is 0 so empty-axis sum is
	// allowed (returns zeros). mean / min / max / argmin / argmax
	// error cleanly on empty axis — no defined identity.
	//
	// keepdims pairs naturally with broadcasting: the result keeps
	// the reduced dimension as size 1, so subtracting/dividing the
	// result back against the original shape works without an
	// explicit expand_dims call. NumPy parallel: keepdims=True.

	// _tensor_sum_axis(t: tensor, axis: int) → tensor
	//
	// Sum along one axis. Result has rank len(t.shape)-1. Output is
	// same dtype as input. Empty axis returns zeros (sum's identity).
	// NumPy parallel: np.sum(t, axis=N).
	Builtins["_tensor_sum_axis"] = &Builtin{Fn: func(args []Object) Object {
		info, errObj := axisReductionPrologue(args, "_tensor_sum_axis", false)
		if errObj != nil {
			return errObj
		}
		return doSumAxisOp(info, info.dropShape)
	}}

	// _tensor_sum_axis_keepdims(t: tensor, axis: int) → tensor
	//
	// Same as _tensor_sum_axis but keeps the reduced axis as size 1
	// in the output shape. Composes cleanly with broadcasting.
	Builtins["_tensor_sum_axis_keepdims"] = &Builtin{Fn: func(args []Object) Object {
		info, errObj := axisReductionPrologue(args, "_tensor_sum_axis_keepdims", false)
		if errObj != nil {
			return errObj
		}
		return doSumAxisOp(info, info.keepShape)
	}}

	// _tensor_mean_axis(t: tensor, axis: int) → tensor
	//
	// Mean along one axis. Always returns f64 dtype (matches scalar
	// mean's policy and NumPy's np.mean on integer arrays). Empty
	// axis errors cleanly.
	Builtins["_tensor_mean_axis"] = &Builtin{Fn: func(args []Object) Object {
		info, errObj := axisReductionPrologue(args, "_tensor_mean_axis", true)
		if errObj != nil {
			return errObj
		}
		return doMeanAxisOp(info, info.dropShape)
	}}

	// _tensor_mean_axis_keepdims(t: tensor, axis: int) → tensor
	//
	// keepdims variant of _tensor_mean_axis. Reduced axis stays at
	// size 1; useful for centring (x - mean(x, axis, keepdims=true)).
	Builtins["_tensor_mean_axis_keepdims"] = &Builtin{Fn: func(args []Object) Object {
		info, errObj := axisReductionPrologue(args, "_tensor_mean_axis_keepdims", true)
		if errObj != nil {
			return errObj
		}
		return doMeanAxisOp(info, info.keepShape)
	}}

	// _tensor_min_axis(t: tensor, axis: int) → tensor
	//
	// Minimum along one axis. Output is same dtype as input. Empty
	// axis errors.
	Builtins["_tensor_min_axis"] = &Builtin{Fn: func(args []Object) Object {
		info, errObj := axisReductionPrologue(args, "_tensor_min_axis", true)
		if errObj != nil {
			return errObj
		}
		return doMinAxisOp(info, info.dropShape)
	}}

	// _tensor_min_axis_keepdims(t: tensor, axis: int) → tensor
	Builtins["_tensor_min_axis_keepdims"] = &Builtin{Fn: func(args []Object) Object {
		info, errObj := axisReductionPrologue(args, "_tensor_min_axis_keepdims", true)
		if errObj != nil {
			return errObj
		}
		return doMinAxisOp(info, info.keepShape)
	}}

	// _tensor_max_axis(t: tensor, axis: int) → tensor
	//
	// Maximum along one axis. Output is same dtype as input. Empty
	// axis errors.
	Builtins["_tensor_max_axis"] = &Builtin{Fn: func(args []Object) Object {
		info, errObj := axisReductionPrologue(args, "_tensor_max_axis", true)
		if errObj != nil {
			return errObj
		}
		return doMaxAxisOp(info, info.dropShape)
	}}

	// _tensor_max_axis_keepdims(t: tensor, axis: int) → tensor
	Builtins["_tensor_max_axis_keepdims"] = &Builtin{Fn: func(args []Object) Object {
		info, errObj := axisReductionPrologue(args, "_tensor_max_axis_keepdims", true)
		if errObj != nil {
			return errObj
		}
		return doMaxAxisOp(info, info.keepShape)
	}}

	// _tensor_argmin_axis(t: tensor, axis: int) → tensor
	//
	// Index of the minimum element along one axis. Indices refer to
	// positions within the reduced axis. Output is i64 dtype
	// regardless of input. First-occurrence on ties. Empty axis
	// errors.
	Builtins["_tensor_argmin_axis"] = &Builtin{Fn: func(args []Object) Object {
		info, errObj := axisReductionPrologue(args, "_tensor_argmin_axis", true)
		if errObj != nil {
			return errObj
		}
		return doArgminAxisOp(info, info.dropShape)
	}}

	// _tensor_argmin_axis_keepdims(t: tensor, axis: int) → tensor
	Builtins["_tensor_argmin_axis_keepdims"] = &Builtin{Fn: func(args []Object) Object {
		info, errObj := axisReductionPrologue(args, "_tensor_argmin_axis_keepdims", true)
		if errObj != nil {
			return errObj
		}
		return doArgminAxisOp(info, info.keepShape)
	}}

	// _tensor_argmax_axis(t: tensor, axis: int) → tensor
	//
	// Index of the maximum element along one axis. Indices refer to
	// positions within the reduced axis. Output is i64 dtype
	// regardless of input. First-occurrence on ties. Empty axis
	// errors.
	Builtins["_tensor_argmax_axis"] = &Builtin{Fn: func(args []Object) Object {
		info, errObj := axisReductionPrologue(args, "_tensor_argmax_axis", true)
		if errObj != nil {
			return errObj
		}
		return doArgmaxAxisOp(info, info.dropShape)
	}}

	// _tensor_argmax_axis_keepdims(t: tensor, axis: int) → tensor
	Builtins["_tensor_argmax_axis_keepdims"] = &Builtin{Fn: func(args []Object) Object {
		info, errObj := axisReductionPrologue(args, "_tensor_argmax_axis_keepdims", true)
		if errObj != nil {
			return errObj
		}
		return doArgmaxAxisOp(info, info.keepShape)
	}}

	// _tensor_reshape(t: tensor, newShape: array) -> tensor
	//
	// Returns a NEW Tensor with the requested shape, sharing the
	// backing data slice with t (NumPy-style view). product(newShape)
	// must equal t.Numel(); otherwise an error is returned.
	//
	// v1 only handles contiguous → contiguous. Non-contiguous inputs
	// (Strides != nil) error cleanly — they'd need materialisation
	// first, which v1's storage layer doesn't yet expose.
	//
	// Because the returned tensor shares storage, in-place ops on
	// either alias mutate both. This matches NumPy's reshape-returns-
	// view semantics. For an independent copy, callers can materialise
	// via an element-wise op (e.g. add a zeros tensor) — clone is a
	// later addition.
	Builtins["_tensor_reshape"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_tensor_reshape expects 2 arguments (tensor, newShape)", ast.Pos{})
		}
		src, ok := args[0].(*Tensor)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_reshape: first argument must be a tensor, got %s", args[0].Type()), ast.Pos{})
		}
		shapeArr, ok := args[1].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_reshape: newShape must be an array, got %s", args[1].Type()), ast.Pos{})
		}
		if !src.IsContiguous() {
			return runtimeError("_tensor_reshape: non-contiguous inputs not yet supported", ast.Pos{})
		}
		newShape, err := tensorShapeFromArray(shapeArr)
		if err != nil {
			return err
		}
		newNumel := 1
		for _, s := range newShape {
			newNumel *= s
		}
		if newNumel != src.Numel() {
			return runtimeError(fmt.Sprintf("_tensor_reshape: numel mismatch — source has %d elements, requested shape %v has %d",
				src.Numel(), newShape, newNumel), ast.Pos{})
		}
		// Share backing slices; Strides==nil preserves contiguous flag.
		return &Tensor{
			DType: src.DType,
			Shape: newShape,
			F32:   src.F32,
			F64:   src.F64,
			I64:   src.I64,
		}
	}}

	// _tensor_squeeze(t: tensor) -> tensor
	//
	// Returns a view of t with all size-1 dimensions removed. If every
	// dimension is 1, the result is a scalar tensor (shape `[]`,
	// numel 1). Shares the backing data slice with t — in-place ops
	// on either alias mutate both.
	//
	// NumPy equivalent: np.squeeze. (NumPy's optional axis= argument
	// is deferred — squeeze-all is the common case.)
	Builtins["_tensor_squeeze"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_tensor_squeeze expects 1 argument (tensor)", ast.Pos{})
		}
		src, ok := args[0].(*Tensor)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_squeeze: argument must be a tensor, got %s", args[0].Type()), ast.Pos{})
		}
		if !src.IsContiguous() {
			return runtimeError("_tensor_squeeze: non-contiguous inputs not yet supported", ast.Pos{})
		}
		newShape := make([]int, 0, len(src.Shape))
		for _, s := range src.Shape {
			if s != 1 {
				newShape = append(newShape, s)
			}
		}
		return &Tensor{
			DType: src.DType,
			Shape: newShape,
			F32:   src.F32,
			F64:   src.F64,
			I64:   src.I64,
		}
	}}

	// _tensor_expand_dims(t: tensor, axis: int) -> tensor
	//
	// Returns a view of t with a new size-1 dimension inserted at the
	// given axis. axis may be negative (counts from the end). Valid
	// range is [-len(shape)-1, len(shape)] — i.e., the axis can be
	// inserted at any existing position OR appended after the last.
	//
	// NumPy equivalent: np.expand_dims.
	//
	//   t.expand_dims(t, 0)       // (3, 4) → (1, 3, 4)
	//   t.expand_dims(t, 1)       // (3, 4) → (3, 1, 4)
	//   t.expand_dims(t, -1)      // (3, 4) → (3, 4, 1)
	Builtins["_tensor_expand_dims"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_tensor_expand_dims expects 2 arguments (tensor, axis)", ast.Pos{})
		}
		src, ok := args[0].(*Tensor)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_expand_dims: first argument must be a tensor, got %s", args[0].Type()), ast.Pos{})
		}
		axisObj, ok := args[1].(*Integer)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_expand_dims: axis must be an integer, got %s", args[1].Type()), ast.Pos{})
		}
		if !src.IsContiguous() {
			return runtimeError("_tensor_expand_dims: non-contiguous inputs not yet supported", ast.Pos{})
		}
		axis := axisObj.Value
		nd := len(src.Shape)
		// Negative axis: -1 means "after the last", -2 means "before
		// the last", etc. Range: [-nd-1, nd].
		if axis < 0 {
			axis = axis + nd + 1
		}
		if axis < 0 || axis > nd {
			return runtimeError(fmt.Sprintf("_tensor_expand_dims: axis %d out of range for tensor with %d dimensions (valid: -%d to %d)",
				axisObj.Value, nd, nd+1, nd), ast.Pos{})
		}
		newShape := make([]int, nd+1)
		copy(newShape[:axis], src.Shape[:axis])
		newShape[axis] = 1
		copy(newShape[axis+1:], src.Shape[axis:])
		return &Tensor{
			DType: src.DType,
			Shape: newShape,
			F32:   src.F32,
			F64:   src.F64,
			I64:   src.I64,
		}
	}}

	// _tensor_dot(a: tensor, b: tensor) -> number
	//
	// 1-D inner product: sum(a[i] * b[i]) for i in [0, n). Both inputs
	// must be 1-D, same dtype, same length, contiguous. Returns a
	// scalar of the matching dtype (Float for f32/f64, Integer for
	// i64). For higher-dimensional dot/matmul patterns, use
	// _tensor_matmul.
	//
	// Implemented as a fused C kernel (mul + sum in one pass) so no
	// temporary tensor is allocated. NumPy equivalent: np.dot for the
	// 1-D × 1-D case.
	Builtins["_tensor_dot"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_tensor_dot expects 2 arguments (a, b)", ast.Pos{})
		}
		a, aOk := args[0].(*Tensor)
		if !aOk {
			return typeError(fmt.Sprintf("_tensor_dot: a must be a tensor, got %s", args[0].Type()), ast.Pos{})
		}
		b, bOk := args[1].(*Tensor)
		if !bOk {
			return typeError(fmt.Sprintf("_tensor_dot: b must be a tensor, got %s", args[1].Type()), ast.Pos{})
		}
		if !tensorComputeAvailable() {
			return runtimeError("tensor ops require macOS or Linux in v1 (Windows support deferred)", ast.Pos{})
		}
		if a.DType != b.DType {
			return typeError(fmt.Sprintf("_tensor_dot: dtype mismatch (%s vs %s)", a.DType, b.DType), ast.Pos{})
		}
		if len(a.Shape) != 1 || len(b.Shape) != 1 {
			return runtimeError(fmt.Sprintf("_tensor_dot: both inputs must be 1-D (got shapes %v and %v); use matmul for 2-D", a.Shape, b.Shape), ast.Pos{})
		}
		if a.Shape[0] != b.Shape[0] {
			return runtimeError(fmt.Sprintf("_tensor_dot: length mismatch (%d vs %d)", a.Shape[0], b.Shape[0]), ast.Pos{})
		}
		if !a.IsContiguous() || !b.IsContiguous() {
			return runtimeError("_tensor_dot: non-contiguous inputs not yet supported", ast.Pos{})
		}
		switch a.DType {
		case DTypeFloat32:
			return &Float{Value: float64(tensorDotF32(a.F32, b.F32))}
		case DTypeFloat64:
			return &Float{Value: tensorDotF64(a.F64, b.F64)}
		case DTypeInt64:
			return intObj(int(tensorDotI64(a.I64, b.I64)))
		}
		return runtimeError("_tensor_dot: unreachable dtype", ast.Pos{})
	}}

	// _tensor_transpose(t: tensor) -> tensor
	//
	// 2-D matrix transpose. Input must be a 2-D contiguous tensor of
	// any supported dtype; result is a fresh contiguous tensor with
	// axes swapped: shape [m, n] → shape [n, m], out[i, j] = in[j, i].
	//
	// Materialising (allocating) rather than view-based — keeps the
	// downstream kernels (matmul, element-wise) simple by not
	// requiring stride-aware variants. Cost is one tensor allocation
	// + a single-pass copy through the C kernel.
	//
	// v1 is 2-D only. N-D transpose (NumPy's axis-permutation form)
	// is deferred until a workload demands it.
	Builtins["_tensor_transpose"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_tensor_transpose expects 1 argument (tensor)", ast.Pos{})
		}
		src, ok := args[0].(*Tensor)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_transpose: argument must be a tensor, got %s", args[0].Type()), ast.Pos{})
		}
		if !tensorComputeAvailable() {
			return runtimeError("tensor ops require macOS or Linux in v1 (Windows support deferred)", ast.Pos{})
		}
		if len(src.Shape) != 2 {
			return runtimeError(fmt.Sprintf("_tensor_transpose: input must be 2-D, got shape %v (N-D transpose deferred to v2)", src.Shape), ast.Pos{})
		}
		if !src.IsContiguous() {
			return runtimeError("_tensor_transpose: non-contiguous inputs not yet supported", ast.Pos{})
		}
		m, n := src.Shape[0], src.Shape[1]
		out := newTensorFromShape(src.DType, []int{n, m})
		switch src.DType {
		case DTypeFloat32:
			tensorTranspose2DF32(out.F32, src.F32, m, n)
		case DTypeFloat64:
			tensorTranspose2DF64(out.F64, src.F64, m, n)
		case DTypeInt64:
			tensorTranspose2DI64(out.I64, src.I64, m, n)
		}
		return out
	}}

	// _tensor_matmul(a: tensor, b: tensor) -> tensor
	//
	// Matrix multiplication C := A · B for 2-D tensors. Shape rules:
	//
	//   a: [m, k]   b: [k, n]   →   c: [m, n]
	//
	// Dispatch:
	//   - f32 on macOS: MPSMatrixMultiplication (GPU). MPS overhead
	//     loses to a naive CPU loop below ~256³; we accept that for
	//     v1 simplicity — the gap is sub-millisecond and the API
	//     stays predictable.
	//   - f32 on Linux: CPU kernel (-O3 -ffast-math autovec).
	//   - f64 / i64: CPU kernel on every platform. MPS is f32-only at
	//     the current bridge surface.
	//
	// Both inputs must be 2-D, same dtype, both contiguous. No
	// broadcasting; no batched matmul in v1.
	Builtins["_tensor_matmul"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_tensor_matmul expects 2 arguments (a, b)", ast.Pos{})
		}
		a, aOk := args[0].(*Tensor)
		if !aOk {
			return typeError(fmt.Sprintf("_tensor_matmul: a must be a tensor, got %s", args[0].Type()), ast.Pos{})
		}
		b, bOk := args[1].(*Tensor)
		if !bOk {
			return typeError(fmt.Sprintf("_tensor_matmul: b must be a tensor, got %s", args[1].Type()), ast.Pos{})
		}
		if !tensorComputeAvailable() {
			return runtimeError("tensor ops require macOS or Linux in v1 (Windows support deferred)", ast.Pos{})
		}
		if a.DType != b.DType {
			return typeError(fmt.Sprintf("_tensor_matmul: dtype mismatch — a is %s, b is %s (FrogPy v1 does not promote)", a.DType, b.DType), ast.Pos{})
		}
		if len(a.Shape) != 2 {
			return runtimeError(fmt.Sprintf("_tensor_matmul: a must be 2-D, got shape %v", a.Shape), ast.Pos{})
		}
		if len(b.Shape) != 2 {
			return runtimeError(fmt.Sprintf("_tensor_matmul: b must be 2-D, got shape %v", b.Shape), ast.Pos{})
		}
		if a.Shape[1] != b.Shape[0] {
			return runtimeError(fmt.Sprintf("_tensor_matmul: shape mismatch — a is %dx%d, b is %dx%d; need a.shape[1] == b.shape[0]",
				a.Shape[0], a.Shape[1], b.Shape[0], b.Shape[1]), ast.Pos{})
		}
		if !a.IsContiguous() {
			return runtimeError("_tensor_matmul: a must be contiguous (non-contiguous inputs not yet supported)", ast.Pos{})
		}
		if !b.IsContiguous() {
			return runtimeError("_tensor_matmul: b must be contiguous (non-contiguous inputs not yet supported)", ast.Pos{})
		}

		m, k, n := a.Shape[0], a.Shape[1], b.Shape[1]
		out := newTensorFromShape(a.DType, []int{m, n})

		switch a.DType {
		case DTypeFloat32:
			handled, errMsg := tensorMatmulMPSf32(out.F32, a.F32, b.F32, m, k, n)
			if handled {
				if errMsg != "" {
					return runtimeError("_tensor_matmul: "+errMsg, ast.Pos{})
				}
				return out
			}
			tensorMatmulF32(out.F32, a.F32, b.F32, m, k, n)
			return out
		case DTypeFloat64:
			tensorMatmulF64(out.F64, a.F64, b.F64, m, k, n)
			return out
		case DTypeInt64:
			tensorMatmulI64(out.I64, a.I64, b.I64, m, k, n)
			return out
		}
		return runtimeError("_tensor_matmul: unreachable dtype", ast.Pos{})
	}}
}

// reductionPrologue runs the shared validation for every reduction
// builtin: 1-arg, must be a Tensor, platform must support tensor ops,
// must be contiguous. If rejectEmpty is true, empty tensors are
// rejected with a clean kLex error — that's used by mean/min/max/
// argmin/argmax. sum sets rejectEmpty=false because the identity is
// well-defined (0 for all dtypes).
//
// Returns (tensor, nil) on success or (nil, *Error) on failure. The
// caller is expected to handle the nil-tensor case by returning the
// error directly.
func reductionPrologue(args []Object, opName string, rejectEmpty bool) (*Tensor, Object) {
	if len(args) != 1 {
		return nil, runtimeError(fmt.Sprintf("%s expects 1 argument", opName), ast.Pos{})
	}
	t, ok := args[0].(*Tensor)
	if !ok {
		return nil, typeError(fmt.Sprintf("%s: argument must be a tensor, got %s", opName, args[0].Type()), ast.Pos{})
	}
	if !tensorComputeAvailable() {
		return nil, runtimeError("tensor ops require macOS or Linux in v1 (Windows support deferred)", ast.Pos{})
	}
	if !t.IsContiguous() {
		return nil, runtimeError(fmt.Sprintf("%s: non-contiguous inputs not yet supported", opName), ast.Pos{})
	}
	if rejectEmpty && t.Numel() == 0 {
		return nil, runtimeError(fmt.Sprintf("%s: input tensor is empty", opName), ast.Pos{})
	}
	return t, nil
}

// axisReduction holds everything the per-op axis reduction builtins
// need: the validated tensor + normalised axis, the (prefix, reduceLen,
// suffix) flattening the C kernels consume, and both candidate output
// shapes so a single prologue serves the drop-shape and keepdims
// variants.
type axisReduction struct {
	t         *Tensor
	axis      int   // normalised axis (0 <= axis < rank)
	prefix    int   // product of dims before axis (1 if axis is leading)
	reduceLen int   // size of the reduced axis (== t.Shape[axis])
	suffix    int   // product of dims after axis (1 if axis is trailing)
	dropShape []int // output shape with the reduced axis removed
	keepShape []int // output shape with the reduced axis kept at size 1
}

// axisReductionPrologue validates an N-D axis reduction call:
//   - args: (tensor, axis)
//   - tensor must have rank >= 1, be contiguous, and a supported dtype
//   - axis is normalised to [0, rank); negative values count from end
//   - if rejectEmpty, errors when t.Shape[axis] == 0
//
// Returns an *axisReduction on success or (nil, *Error) on failure.
func axisReductionPrologue(args []Object, opName string, rejectEmpty bool) (*axisReduction, Object) {
	if len(args) != 2 {
		return nil, runtimeError(fmt.Sprintf("%s expects 2 arguments (tensor, axis)", opName), ast.Pos{})
	}
	t, ok := args[0].(*Tensor)
	if !ok {
		return nil, typeError(fmt.Sprintf("%s: first argument must be a tensor, got %s", opName, args[0].Type()), ast.Pos{})
	}
	axisObj, ok := args[1].(*Integer)
	if !ok {
		return nil, typeError(fmt.Sprintf("%s: axis must be an integer, got %s", opName, args[1].Type()), ast.Pos{})
	}
	if !tensorComputeAvailable() {
		return nil, runtimeError("tensor ops require macOS or Linux in v1 (Windows support deferred)", ast.Pos{})
	}
	rank := len(t.Shape)
	if rank < 1 {
		return nil, runtimeError(fmt.Sprintf("%s: input must have rank >= 1 (got scalar tensor)", opName), ast.Pos{})
	}
	if !t.IsContiguous() {
		return nil, runtimeError(fmt.Sprintf("%s: non-contiguous inputs not yet supported", opName), ast.Pos{})
	}
	axis := axisObj.Value
	if axis < 0 {
		axis = axis + rank
	}
	if axis < 0 || axis >= rank {
		return nil, runtimeError(fmt.Sprintf("%s: axis %d out of range for shape %v (valid: [%d, %d])", opName, axisObj.Value, t.Shape, -rank, rank-1), ast.Pos{})
	}
	reduceLen := t.Shape[axis]
	if rejectEmpty && reduceLen == 0 {
		return nil, runtimeError(fmt.Sprintf("%s: cannot reduce empty axis %d (shape %v)", opName, axis, t.Shape), ast.Pos{})
	}
	prefix := 1
	for i := 0; i < axis; i++ {
		prefix *= t.Shape[i]
	}
	suffix := 1
	for i := axis + 1; i < rank; i++ {
		suffix *= t.Shape[i]
	}
	dropShape := make([]int, 0, rank-1)
	for i, s := range t.Shape {
		if i != axis {
			dropShape = append(dropShape, s)
		}
	}
	keepShape := make([]int, rank)
	for i, s := range t.Shape {
		if i == axis {
			keepShape[i] = 1
		} else {
			keepShape[i] = s
		}
	}
	return &axisReduction{
		t:         t,
		axis:      axis,
		prefix:    prefix,
		reduceLen: reduceLen,
		suffix:    suffix,
		dropShape: dropShape,
		keepShape: keepShape,
	}, nil
}

// doSumAxisOp dispatches the per-dtype sum-along-axis kernel and
// returns a tensor of the requested output shape (drop or keepdims).
// Output dtype matches input. Empty axis returns a zero-filled tensor
// (newTensorFromShape zeros the backing slice).
func doSumAxisOp(info *axisReduction, outShape []int) Object {
	out := newTensorFromShape(info.t.DType, outShape)
	if info.reduceLen == 0 {
		return out
	}
	switch info.t.DType {
	case DTypeFloat32:
		tensorSumAxisF32(out.F32, info.t.F32, info.prefix, info.reduceLen, info.suffix)
	case DTypeFloat64:
		tensorSumAxisF64(out.F64, info.t.F64, info.prefix, info.reduceLen, info.suffix)
	case DTypeInt64:
		tensorSumAxisI64(out.I64, info.t.I64, info.prefix, info.reduceLen, info.suffix)
	}
	return out
}

// doMeanAxisOp computes per-axis mean via the sum kernel + Go-side
// division. Output is always f64 to match scalar mean's precision
// policy. Caller's rejectEmpty=true in the prologue guarantees
// reduceLen >= 1 so denom is never zero.
func doMeanAxisOp(info *axisReduction, outShape []int) Object {
	out := newTensorFromShape(DTypeFloat64, outShape)
	denom := float64(info.reduceLen)
	switch info.t.DType {
	case DTypeFloat32:
		tmp := make([]float32, len(out.F64))
		tensorSumAxisF32(tmp, info.t.F32, info.prefix, info.reduceLen, info.suffix)
		for i, v := range tmp {
			out.F64[i] = float64(v) / denom
		}
	case DTypeFloat64:
		tensorSumAxisF64(out.F64, info.t.F64, info.prefix, info.reduceLen, info.suffix)
		for i := range out.F64 {
			out.F64[i] /= denom
		}
	case DTypeInt64:
		tmp := make([]int64, len(out.F64))
		tensorSumAxisI64(tmp, info.t.I64, info.prefix, info.reduceLen, info.suffix)
		for i, v := range tmp {
			out.F64[i] = float64(v) / denom
		}
	}
	return out
}

func doMinAxisOp(info *axisReduction, outShape []int) Object {
	out := newTensorFromShape(info.t.DType, outShape)
	switch info.t.DType {
	case DTypeFloat32:
		tensorMinAxisF32(out.F32, info.t.F32, info.prefix, info.reduceLen, info.suffix)
	case DTypeFloat64:
		tensorMinAxisF64(out.F64, info.t.F64, info.prefix, info.reduceLen, info.suffix)
	case DTypeInt64:
		tensorMinAxisI64(out.I64, info.t.I64, info.prefix, info.reduceLen, info.suffix)
	}
	return out
}

func doMaxAxisOp(info *axisReduction, outShape []int) Object {
	out := newTensorFromShape(info.t.DType, outShape)
	switch info.t.DType {
	case DTypeFloat32:
		tensorMaxAxisF32(out.F32, info.t.F32, info.prefix, info.reduceLen, info.suffix)
	case DTypeFloat64:
		tensorMaxAxisF64(out.F64, info.t.F64, info.prefix, info.reduceLen, info.suffix)
	case DTypeInt64:
		tensorMaxAxisI64(out.I64, info.t.I64, info.prefix, info.reduceLen, info.suffix)
	}
	return out
}

func doArgminAxisOp(info *axisReduction, outShape []int) Object {
	out := newTensorFromShape(DTypeInt64, outShape)
	switch info.t.DType {
	case DTypeFloat32:
		tensorArgminAxisF32(out.I64, info.t.F32, info.prefix, info.reduceLen, info.suffix)
	case DTypeFloat64:
		tensorArgminAxisF64(out.I64, info.t.F64, info.prefix, info.reduceLen, info.suffix)
	case DTypeInt64:
		tensorArgminAxisI64(out.I64, info.t.I64, info.prefix, info.reduceLen, info.suffix)
	}
	return out
}

func doArgmaxAxisOp(info *axisReduction, outShape []int) Object {
	out := newTensorFromShape(DTypeInt64, outShape)
	switch info.t.DType {
	case DTypeFloat32:
		tensorArgmaxAxisF32(out.I64, info.t.F32, info.prefix, info.reduceLen, info.suffix)
	case DTypeFloat64:
		tensorArgmaxAxisF64(out.I64, info.t.F64, info.prefix, info.reduceLen, info.suffix)
	case DTypeInt64:
		tensorArgmaxAxisI64(out.I64, info.t.I64, info.prefix, info.reduceLen, info.suffix)
	}
	return out
}

// ── helpers ──

// binaryKernel bundles the three dtype-specific kernel pointers for
// one element-wise binary op (add, sub, mul, div, etc.).
//
// preCheck is an optional validation step that runs AFTER shape/
// dtype/contiguity checks but BEFORE any kernel dispatch. div uses
// it to scan the i64 divisor for zeros. Returns "" on success or a
// kLex error message string.
type binaryKernel struct {
	f32      func(out, a, b []float32)
	f64      func(out, a, b []float64)
	i64      func(out, a, b []int64)
	preCheck func(a, b *Tensor) string
}

// elementWiseBinary is the shared dispatch helper for every binary
// element-wise tensor op. Centralising the shape/dtype/contiguity
// validation here means each builtin (_tensor_add, _tensor_sub,
// etc.) is a one-liner — and the rules can't drift between ops as
// the surface grows.
//
// Accepts:
//   - tensor op tensor (NumPy broadcasting rules apply)
//   - tensor op scalar (scalar materialised to tensor's shape + dtype)
//   - scalar op tensor (symmetric)
//
// Broadcasting follows NumPy semantics (see broadcastShape below).
// The smaller operand is MATERIALISED into a fresh tensor at the
// broadcast shape — this trades allocation for kernel simplicity
// (no per-op broadcast-aware C variants).
//
// Allocating variants only — in-place ops (elementWiseBinaryInplace)
// stay strict (same shape, same dtype) so the caller's tensor isn't
// silently resized.
func elementWiseBinary(args []Object, opName string, k binaryKernel) Object {
	if len(args) != 2 {
		return runtimeError(fmt.Sprintf("%s expects 2 arguments (a, b)", opName), ast.Pos{})
	}
	if !tensorComputeAvailable() {
		return runtimeError("tensor ops require macOS or Linux in v1 (Windows support deferred)", ast.Pos{})
	}

	aTen, aIsT := args[0].(*Tensor)
	bTen, bIsT := args[1].(*Tensor)
	if !aIsT && !bIsT {
		return typeError(fmt.Sprintf("%s: at least one argument must be a tensor (got %s and %s)", opName, args[0].Type(), args[1].Type()), ast.Pos{})
	}

	// Anchor dtype on whichever side is a tensor. If both are tensors
	// the dtype check below catches a mismatch.
	var ref *Tensor
	if aIsT {
		ref = aTen
	} else {
		ref = bTen
	}

	// Promote scalars to ref-shaped tensors. The materialised tensor
	// inherits the reference's dtype; this is where i64+Float would
	// be caught with a clean error.
	if !aIsT {
		mat, errObj := tensorFromScalar(args[0], ref)
		if errObj != nil {
			return errObj
		}
		aTen = mat
	}
	if !bIsT {
		mat, errObj := tensorFromScalar(args[1], ref)
		if errObj != nil {
			return errObj
		}
		bTen = mat
	}

	if aTen.DType != bTen.DType {
		return typeError(fmt.Sprintf("%s: dtype mismatch (%s vs %s); explicit conversion required", opName, aTen.DType, bTen.DType), ast.Pos{})
	}
	if !aTen.IsContiguous() || !bTen.IsContiguous() {
		return runtimeError(fmt.Sprintf("%s: non-contiguous inputs not yet supported", opName), ast.Pos{})
	}

	outShape, bcErr := broadcastShape(aTen.Shape, bTen.Shape)
	if bcErr != "" {
		return runtimeError(fmt.Sprintf("%s: %s", opName, bcErr), ast.Pos{})
	}

	// Tile either operand to the broadcast shape if it doesn't
	// already match. materializeBroadcast is a no-op when shapes
	// are equal.
	if !sameShape(aTen.Shape, outShape) {
		aTen = materializeBroadcast(aTen, outShape)
	}
	if !sameShape(bTen.Shape, outShape) {
		bTen = materializeBroadcast(bTen, outShape)
	}

	if k.preCheck != nil {
		if msg := k.preCheck(aTen, bTen); msg != "" {
			return runtimeError(fmt.Sprintf("%s: %s", opName, msg), ast.Pos{})
		}
	}
	out := newTensorFromShape(aTen.DType, outShape)
	switch aTen.DType {
	case DTypeFloat32:
		if k.f32 == nil {
			return typeError(fmt.Sprintf("%s: not supported for f32", opName), ast.Pos{})
		}
		k.f32(out.F32, aTen.F32, bTen.F32)
	case DTypeFloat64:
		if k.f64 == nil {
			return typeError(fmt.Sprintf("%s: not supported for f64", opName), ast.Pos{})
		}
		k.f64(out.F64, aTen.F64, bTen.F64)
	case DTypeInt64:
		if k.i64 == nil {
			return typeError(fmt.Sprintf("%s: not supported for i64", opName), ast.Pos{})
		}
		k.i64(out.I64, aTen.I64, bTen.I64)
	}
	return out
}

// broadcastShape returns the NumPy broadcast result of two shape
// vectors, or a non-empty error string if the shapes can't be
// broadcast together. Rules:
//
//   1. Align shapes from the trailing (rightmost) dimension.
//   2. Missing leading dimensions on either side are treated as 1.
//   3. Two dims are compatible if they're equal OR one is 1.
//   4. Result takes the max of each dimension.
//
// Examples:
//   (3, 4)  + (4,)    → (3, 4)
//   (3, 1)  + (1, 4)  → (3, 4)
//   (5, 3, 4) + (3, 1)→ (5, 3, 4)
//   (3, 4)  + (3, 5)  → error
func broadcastShape(aShape, bShape []int) ([]int, string) {
	nd := len(aShape)
	if len(bShape) > nd {
		nd = len(bShape)
	}
	out := make([]int, nd)
	for i := 0; i < nd; i++ {
		aIdx := len(aShape) - 1 - i
		bIdx := len(bShape) - 1 - i
		outIdx := nd - 1 - i
		ad, bd := 1, 1
		if aIdx >= 0 {
			ad = aShape[aIdx]
		}
		if bIdx >= 0 {
			bd = bShape[bIdx]
		}
		switch {
		case ad == bd:
			out[outIdx] = ad
		case ad == 1:
			out[outIdx] = bd
		case bd == 1:
			out[outIdx] = ad
		default:
			return nil, fmt.Sprintf("operands could not be broadcast together with shapes %v and %v", aShape, bShape)
		}
	}
	return out, ""
}

// materializeBroadcast tiles t's data into a fresh contiguous tensor
// of targetShape. The two shapes must be broadcast-compatible AND
// targetShape must be the broadcast result (caller's responsibility
// via broadcastShape).
//
// If t already has targetShape, returns t unchanged (no allocation).
func materializeBroadcast(t *Tensor, targetShape []int) *Tensor {
	if sameShape(t.Shape, targetShape) {
		return t
	}
	out := newTensorFromShape(t.DType, targetShape)
	nd := len(targetShape)

	// Pad input shape with leading 1s to match output rank.
	paddedShape := make([]int, nd)
	for i := 0; i < nd; i++ {
		srcIdx := len(t.Shape) - (nd - i)
		if srcIdx >= 0 {
			paddedShape[i] = t.Shape[srcIdx]
		} else {
			paddedShape[i] = 1
		}
	}

	// Row-major strides for both shapes. Input strides are computed
	// from the padded shape so they match the output's rank.
	inStrides := make([]int, nd)
	outStrides := make([]int, nd)
	inStride, outStride := 1, 1
	for i := nd - 1; i >= 0; i-- {
		inStrides[i] = inStride
		outStrides[i] = outStride
		inStride *= paddedShape[i]
		outStride *= targetShape[i]
	}

	idx := make([]int, nd)
	targetNumel := out.Numel()
	for flat := 0; flat < targetNumel; flat++ {
		// Decompose flat output index into per-axis indices.
		r := flat
		for i := 0; i < nd; i++ {
			idx[i] = r / outStrides[i]
			r %= outStrides[i]
		}
		// Compute corresponding input flat index. Axes where the
		// input dim is 1 collapse to position 0 (the broadcast
		// rule); other axes pass through unchanged.
		inIdx := 0
		for i := 0; i < nd; i++ {
			if paddedShape[i] == 1 {
				continue
			}
			inIdx += idx[i] * inStrides[i]
		}
		switch t.DType {
		case DTypeFloat32:
			out.F32[flat] = t.F32[inIdx]
		case DTypeFloat64:
			out.F64[flat] = t.F64[inIdx]
		case DTypeInt64:
			out.I64[flat] = t.I64[inIdx]
		}
	}
	return out
}

// tensorFromScalar produces a same-shape, same-dtype tensor filled
// with `value` (must be Integer or Float). Used by elementWiseBinary
// to promote a scalar operand into a tensor before the kernel call.
//
// Dtype conversion rules mirror _tensor_full:
//   - f32 / f64 accept Integer or Float
//   - i64 accepts Integer only; Float is rejected
func tensorFromScalar(value Object, ref *Tensor) (*Tensor, Object) {
	out := newTensorFromShape(ref.DType, ref.Shape)
	switch ref.DType {
	case DTypeFloat32:
		var v float32
		switch x := value.(type) {
		case *Integer:
			v = float32(x.Value)
		case *Float:
			v = float32(x.Value)
		default:
			return nil, typeError(fmt.Sprintf("cannot broadcast %s into f32 tensor", value.Type()), ast.Pos{})
		}
		for i := range out.F32 {
			out.F32[i] = v
		}
	case DTypeFloat64:
		var v float64
		switch x := value.(type) {
		case *Integer:
			v = float64(x.Value)
		case *Float:
			v = x.Value
		default:
			return nil, typeError(fmt.Sprintf("cannot broadcast %s into f64 tensor", value.Type()), ast.Pos{})
		}
		for i := range out.F64 {
			out.F64[i] = v
		}
	case DTypeInt64:
		var v int64
		switch x := value.(type) {
		case *Integer:
			v = int64(x.Value)
		case *Float:
			return nil, typeError("cannot broadcast Float scalar into i64 tensor (use an integer or change dtype)", ast.Pos{})
		default:
			return nil, typeError(fmt.Sprintf("cannot broadcast %s into i64 tensor", value.Type()), ast.Pos{})
		}
		for i := range out.I64 {
			out.I64[i] = v
		}
	}
	return out, nil
}

// unaryKernel bundles dtype-specific kernel pointers for one
// element-wise unary op. Float-only ops (exp, log, sqrt, sin, cos)
// leave the i64 field nil — elementWiseUnary surfaces a clean
// "not supported for i64" error if a user passes an i64 tensor.
type unaryKernel struct {
	f32      func(out, a []float32)
	f64      func(out, a []float64)
	i64      func(out, a []int64)
	preCheck func(a *Tensor) string
}

// elementWiseUnary is the unary counterpart to elementWiseBinary —
// same shape-checks, same contiguity rule, same dtype dispatch.
// Most unary ops have no preCheck; it's there for future use
// (e.g. domain checks if we ever want to reject negative inputs to
// log eagerly, though current convention is to let IEEE NaN handle
// those silently).
func elementWiseUnary(args []Object, opName string, k unaryKernel) Object {
	if len(args) != 1 {
		return runtimeError(fmt.Sprintf("%s expects 1 argument", opName), ast.Pos{})
	}
	a, ok := args[0].(*Tensor)
	if !ok {
		return typeError(fmt.Sprintf("%s: argument must be a tensor, got %s", opName, args[0].Type()), ast.Pos{})
	}
	if !tensorComputeAvailable() {
		return runtimeError("tensor ops require macOS or Linux in v1 (Windows support deferred)", ast.Pos{})
	}
	if !a.IsContiguous() {
		return runtimeError(fmt.Sprintf("%s: non-contiguous inputs not yet supported", opName), ast.Pos{})
	}
	if k.preCheck != nil {
		if msg := k.preCheck(a); msg != "" {
			return runtimeError(fmt.Sprintf("%s: %s", opName, msg), ast.Pos{})
		}
	}
	out := newTensorFromShape(a.DType, a.Shape)
	switch a.DType {
	case DTypeFloat32:
		if k.f32 == nil {
			return typeError(fmt.Sprintf("%s: not supported for f32", opName), ast.Pos{})
		}
		k.f32(out.F32, a.F32)
	case DTypeFloat64:
		if k.f64 == nil {
			return typeError(fmt.Sprintf("%s: not supported for f64", opName), ast.Pos{})
		}
		k.f64(out.F64, a.F64)
	case DTypeInt64:
		if k.i64 == nil {
			return typeError(fmt.Sprintf("%s: not supported for i64 (use f32 or f64)", opName), ast.Pos{})
		}
		k.i64(out.I64, a.I64)
	}
	return out
}

// elementWiseBinaryInplace is the in-place counterpart to
// elementWiseBinary. Same validation rules — both args must be
// tensors of matching dtype, shape, and contiguity — but instead
// of allocating a fresh output tensor it writes the result back
// into `a`. The C kernels documented aliasing rules permit
// out == a, so we just pass a.F* as both the destination and the
// first source argument.
//
// Returns `a` (the same *Tensor reference) on success so users can
// chain calls. Returns a *Error on validation failure without
// touching `a`'s contents.
//
// Why this exists: the FrogPy diagnostic on 2026-05-23 measured
// the kernel itself at 119 GB/s for f64 add on M4, while the
// allocating variant peaked at 79 GB/s — a 33% drop entirely
// attributable to make([]float64, n)'s zero-init writing every
// byte that the kernel was about to overwrite. The in-place
// variant skips both the alloc and the zero-init.
func elementWiseBinaryInplace(args []Object, opName string, k binaryKernel) Object {
	if len(args) != 2 {
		return runtimeError(fmt.Sprintf("%s expects 2 arguments (a, b)", opName), ast.Pos{})
	}
	a, ok := args[0].(*Tensor)
	if !ok {
		return typeError(fmt.Sprintf("%s: first argument must be a tensor, got %s", opName, args[0].Type()), ast.Pos{})
	}
	b, ok := args[1].(*Tensor)
	if !ok {
		return typeError(fmt.Sprintf("%s: second argument must be a tensor, got %s", opName, args[1].Type()), ast.Pos{})
	}
	if !tensorComputeAvailable() {
		return runtimeError("tensor ops require macOS or Linux in v1 (Windows support deferred)", ast.Pos{})
	}
	if a.DType != b.DType {
		return typeError(fmt.Sprintf("%s: dtype mismatch (%s vs %s); explicit conversion required", opName, a.DType, b.DType), ast.Pos{})
	}
	if !sameShape(a.Shape, b.Shape) {
		return runtimeError(fmt.Sprintf("%s: shape mismatch %v vs %v (broadcasting not yet supported)", opName, a.Shape, b.Shape), ast.Pos{})
	}
	if !a.IsContiguous() || !b.IsContiguous() {
		return runtimeError(fmt.Sprintf("%s: non-contiguous inputs not yet supported", opName), ast.Pos{})
	}
	if k.preCheck != nil {
		if msg := k.preCheck(a, b); msg != "" {
			return runtimeError(fmt.Sprintf("%s: %s", opName, msg), ast.Pos{})
		}
	}
	switch a.DType {
	case DTypeFloat32:
		if k.f32 == nil {
			return typeError(fmt.Sprintf("%s: not supported for f32", opName), ast.Pos{})
		}
		k.f32(a.F32, a.F32, b.F32)
	case DTypeFloat64:
		if k.f64 == nil {
			return typeError(fmt.Sprintf("%s: not supported for f64", opName), ast.Pos{})
		}
		k.f64(a.F64, a.F64, b.F64)
	case DTypeInt64:
		if k.i64 == nil {
			return typeError(fmt.Sprintf("%s: not supported for i64", opName), ast.Pos{})
		}
		k.i64(a.I64, a.I64, b.I64)
	}
	return a
}

// elementWiseUnaryInplace is the unary counterpart to
// elementWiseBinaryInplace. Same validation as elementWiseUnary
// but writes the kernel output back into `a` instead of a fresh
// tensor. Returns `a` on success for chaining.
func elementWiseUnaryInplace(args []Object, opName string, k unaryKernel) Object {
	if len(args) != 1 {
		return runtimeError(fmt.Sprintf("%s expects 1 argument", opName), ast.Pos{})
	}
	a, ok := args[0].(*Tensor)
	if !ok {
		return typeError(fmt.Sprintf("%s: argument must be a tensor, got %s", opName, args[0].Type()), ast.Pos{})
	}
	if !tensorComputeAvailable() {
		return runtimeError("tensor ops require macOS or Linux in v1 (Windows support deferred)", ast.Pos{})
	}
	if !a.IsContiguous() {
		return runtimeError(fmt.Sprintf("%s: non-contiguous inputs not yet supported", opName), ast.Pos{})
	}
	if k.preCheck != nil {
		if msg := k.preCheck(a); msg != "" {
			return runtimeError(fmt.Sprintf("%s: %s", opName, msg), ast.Pos{})
		}
	}
	switch a.DType {
	case DTypeFloat32:
		if k.f32 == nil {
			return typeError(fmt.Sprintf("%s: not supported for f32", opName), ast.Pos{})
		}
		k.f32(a.F32, a.F32)
	case DTypeFloat64:
		if k.f64 == nil {
			return typeError(fmt.Sprintf("%s: not supported for f64", opName), ast.Pos{})
		}
		k.f64(a.F64, a.F64)
	case DTypeInt64:
		if k.i64 == nil {
			return typeError(fmt.Sprintf("%s: not supported for i64 (use f32 or f64)", opName), ast.Pos{})
		}
		k.i64(a.I64, a.I64)
	}
	return a
}

// tensorShapeFromArray validates that the input kLex array is a
// list of non-negative integers and returns the corresponding
// []int. Negative or non-integer entries surface as a runtime
// error.
func tensorShapeFromArray(arr *Array) ([]int, Object) {
	shape := make([]int, len(arr.Elements))
	for i, el := range arr.Elements {
		n, ok := el.(*Integer)
		if !ok {
			return nil, typeError(fmt.Sprintf("shape entry %d must be int, got %s", i, el.Type()), ast.Pos{})
		}
		if n.Value < 0 {
			return nil, runtimeError(fmt.Sprintf("shape entry %d is negative: %d", i, n.Value), ast.Pos{})
		}
		shape[i] = n.Value
	}
	return shape, nil
}

// tensorStoreScalar writes a single kLex Object into a tensor's
// linear index, performing the dtype-appropriate conversion. Returns
// a kLex *Error if the value can't be represented in the dtype.
func tensorStoreScalar(t *Tensor, i int, v Object) Object {
	switch t.DType {
	case DTypeFloat32:
		switch x := v.(type) {
		case *Integer:
			t.F32[i] = float32(x.Value)
		case *Float:
			t.F32[i] = float32(x.Value)
		default:
			return typeError(fmt.Sprintf("cannot store %s into f32 tensor", v.Type()), ast.Pos{})
		}
	case DTypeFloat64:
		switch x := v.(type) {
		case *Integer:
			t.F64[i] = float64(x.Value)
		case *Float:
			t.F64[i] = x.Value
		default:
			return typeError(fmt.Sprintf("cannot store %s into f64 tensor", v.Type()), ast.Pos{})
		}
	case DTypeInt64:
		switch x := v.(type) {
		case *Integer:
			t.I64[i] = int64(x.Value)
		case *Float:
			return typeError("cannot store float into i64 tensor without explicit conversion", ast.Pos{})
		default:
			return typeError(fmt.Sprintf("cannot store %s into i64 tensor", v.Type()), ast.Pos{})
		}
	}
	return nil
}

// sameShape reports whether two shape vectors are equal element-wise.
// Trivial helper but kept named to make call sites read clearly.
func sameShape(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ── Group 1: to_array / clone / cast ────────────────────────────────

func init() {
	// _tensor_to_array(t: tensor) -> array
	//
	// Extracts all elements from t into a flat kLex array, in
	// row-major logical order. Elements are returned as Float for
	// f32/f64 tensors and as Integer for i64 tensors, matching the
	// return convention of _tensor_get.
	//
	// Works on strided views (from _tensor_slice): the multi-index is
	// walked through Strides to fetch each element from the right
	// physical position in the backing slice.
	Builtins["_tensor_to_array"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_tensor_to_array expects 1 argument (tensor)", ast.Pos{})
		}
		t, ok := args[0].(*Tensor)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_to_array: argument must be a tensor, got %s", args[0].Type()), ast.Pos{})
		}
		n := t.Numel()
		els := make([]Object, n)
		if t.IsContiguous() {
			switch t.DType {
			case DTypeFloat32:
				for i := range els {
					els[i] = &Float{Value: float64(t.F32[i])}
				}
			case DTypeFloat64:
				for i := range els {
					els[i] = &Float{Value: t.F64[i]}
				}
			case DTypeInt64:
				for i := range els {
					els[i] = intObj(int(t.I64[i]))
				}
			}
		} else {
			// Strided — materialise once into a contiguous buffer
			// (avoids per-element stride re-walks while extracting).
			mat := materialiseStrided(t)
			switch mat.DType {
			case DTypeFloat32:
				for i := range els {
					els[i] = &Float{Value: float64(mat.F32[i])}
				}
			case DTypeFloat64:
				for i := range els {
					els[i] = &Float{Value: mat.F64[i]}
				}
			case DTypeInt64:
				for i := range els {
					els[i] = intObj(int(mat.I64[i]))
				}
			}
		}
		return &Array{Elements: els}
	}}

	// _tensor_clone(t: tensor) -> tensor
	//
	// Returns a fresh independent copy of t. The backing data is fully
	// copied — mutations to the result do not affect t and vice versa.
	// Use this when you need an independent snapshot of a tensor that
	// may otherwise be aliased by reshape / flatten / expand_dims views,
	// or to materialise a strided view from _tensor_slice into a fresh
	// contiguous tensor.
	Builtins["_tensor_clone"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_tensor_clone expects 1 argument (tensor)", ast.Pos{})
		}
		src, ok := args[0].(*Tensor)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_clone: argument must be a tensor, got %s", args[0].Type()), ast.Pos{})
		}
		// Strided view? Walk the multi-index and produce a fresh
		// contiguous output. Always allocates fresh (vs the no-copy
		// fast path of _tensor_contiguous).
		if !src.IsContiguous() {
			return materialiseStrided(src)
		}
		shapeCopy := make([]int, len(src.Shape))
		copy(shapeCopy, src.Shape)
		out := &Tensor{DType: src.DType, Shape: shapeCopy}
		n := src.Numel()
		switch src.DType {
		case DTypeFloat32:
			out.F32 = make([]float32, n)
			copy(out.F32, src.F32[:n])
		case DTypeFloat64:
			out.F64 = make([]float64, n)
			copy(out.F64, src.F64[:n])
		case DTypeInt64:
			out.I64 = make([]int64, n)
			copy(out.I64, src.I64[:n])
		}
		return out
	}}

	// _tensor_slice(t: tensor, specs: array) -> tensor
	//
	// Returns a VIEW into t — a new tensor with its own Shape/Strides
	// but sharing the same backing data slice. Mutations through the
	// view affect t and vice versa.
	//
	// `specs` is an array with one entry per axis of t (so its length
	// must equal len(t.shape)). Each entry is either:
	//
	//   null                     — take all of this axis (== [null, null, null])
	//   [start, stop, step]      — start/stop/step like Python slice
	//                              (each of start/stop/step may itself be null
	//                              to take the default: 0 / dim / 1)
	//
	// Negative start/stop count from the end of the axis. step must be
	// positive (negative-step / reversed slicing deferred to a later
	// pass). Out-of-bounds start/stop clamp; if start >= stop after
	// normalisation the corresponding axis becomes size 0.
	//
	// Downstream kernels (add, matmul, reductions, etc.) require
	// contiguous input — call `_tensor_contiguous(view)` to materialise
	// before passing a slice to them. _tensor_get, _tensor_to_array,
	// _tensor_clone, _tensor_shape/dtype/numel all work on views
	// directly.
	//
	// v1 limitations: source tensor must itself be contiguous (no
	// slicing of slices in one step — call contiguous() in between).
	Builtins["_tensor_slice"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_tensor_slice expects 2 arguments (tensor, specs)", ast.Pos{})
		}
		src, ok := args[0].(*Tensor)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_slice: first argument must be a tensor, got %s", args[0].Type()), ast.Pos{})
		}
		specsArr, ok := args[1].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_slice: specs must be an array, got %s", args[1].Type()), ast.Pos{})
		}
		if !src.IsContiguous() {
			return runtimeError("_tensor_slice: source tensor is already a view; call t.contiguous() first to materialise before slicing again", ast.Pos{})
		}
		rank := len(src.Shape)
		if len(specsArr.Elements) != rank {
			return runtimeError(fmt.Sprintf("_tensor_slice: specs has %d entries but tensor is rank %d (need one spec per axis)", len(specsArr.Elements), rank), ast.Pos{})
		}
		if rank == 0 {
			return runtimeError("_tensor_slice: cannot slice a scalar tensor (rank 0)", ast.Pos{})
		}

		naturalStrides := computeContiguousStrides(src.Shape)
		newShape := make([]int, rank)
		newStrides := make([]int, rank)
		baseOffset := 0
		for k, specObj := range specsArr.Elements {
			dim := src.Shape[k]
			start, stop, step, errObj := parseSliceSpec(specObj, dim, k)
			if errObj != nil {
				return errObj
			}
			// Normalise negative.
			if start < 0 {
				start += dim
				if start < 0 {
					start = 0
				}
			}
			if stop < 0 {
				stop += dim
				if stop < 0 {
					stop = 0
				}
			}
			// Clamp.
			if start > dim {
				start = dim
			}
			if stop > dim {
				stop = dim
			}
			if start > stop {
				start = stop
			}
			newDim := 0
			if stop > start {
				newDim = (stop - start + step - 1) / step
			}
			newShape[k] = newDim
			newStrides[k] = step * naturalStrides[k]
			baseOffset += start * naturalStrides[k]
		}

		out := &Tensor{DType: src.DType, Shape: newShape, Strides: newStrides}
		switch src.DType {
		case DTypeFloat32:
			if baseOffset > len(src.F32) {
				baseOffset = len(src.F32)
			}
			out.F32 = src.F32[baseOffset:]
		case DTypeFloat64:
			if baseOffset > len(src.F64) {
				baseOffset = len(src.F64)
			}
			out.F64 = src.F64[baseOffset:]
		case DTypeInt64:
			if baseOffset > len(src.I64) {
				baseOffset = len(src.I64)
			}
			out.I64 = src.I64[baseOffset:]
		}
		return out
	}}

	// _tensor_contiguous(t: tensor) -> tensor
	//
	// If t is already contiguous, returns t unchanged (no allocation —
	// matches NumPy's np.ascontiguousarray fast path). If t is a
	// strided view (e.g. produced by _tensor_slice) walks the multi-
	// index and copies into a fresh contiguous tensor.
	//
	// Required before passing a slice view to any kernel-based op
	// (add, matmul, reductions, etc.) since those assume
	// contiguous backing layout. Pair with _tensor_slice for the
	// "view, then materialise" workflow.
	Builtins["_tensor_contiguous"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_tensor_contiguous expects 1 argument (tensor)", ast.Pos{})
		}
		src, ok := args[0].(*Tensor)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_contiguous: argument must be a tensor, got %s", args[0].Type()), ast.Pos{})
		}
		return materialiseStrided(src)
	}}

	// _tensor_cast(t: tensor, dtype: string) -> tensor
	//
	// Returns a new tensor with every element converted to the target dtype.
	// When src and target dtypes are identical, this is equivalent to clone.
	//
	// Conversion rules:
	//   f32 → f64 : lossless widening
	//   f64 → f32 : truncating (silent precision loss for large values)
	//   i64 → f32 : best-effort (exact for |x| ≤ 2^24; silent rounding beyond)
	//   i64 → f64 : near-lossless (exact for |x| ≤ 2^53)
	//   f32 → i64 : truncate toward zero (IEEE floor-to-zero)
	//   f64 → i64 : truncate toward zero
	Builtins["_tensor_cast"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_tensor_cast expects 2 arguments (tensor, dtype)", ast.Pos{})
		}
		src, ok := args[0].(*Tensor)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_cast: first argument must be a tensor, got %s", args[0].Type()), ast.Pos{})
		}
		dtypeStr, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_cast: dtype must be a string, got %s", args[1].Type()), ast.Pos{})
		}
		dt, dok := dtypeFromName(dtypeStr.Value)
		if !dok {
			return typeError(fmt.Sprintf("_tensor_cast: unknown dtype %q (want f32/f64/i64)", dtypeStr.Value), ast.Pos{})
		}
		if dt == src.DType {
			// same dtype — return a clone
			shapeCopy := make([]int, len(src.Shape))
			copy(shapeCopy, src.Shape)
			out := &Tensor{DType: src.DType, Shape: shapeCopy}
			switch src.DType {
			case DTypeFloat32:
				out.F32 = make([]float32, len(src.F32))
				copy(out.F32, src.F32)
			case DTypeFloat64:
				out.F64 = make([]float64, len(src.F64))
				copy(out.F64, src.F64)
			case DTypeInt64:
				out.I64 = make([]int64, len(src.I64))
				copy(out.I64, src.I64)
			}
			return out
		}
		n := src.Numel()
		out := newTensorFromShape(dt, src.Shape)
		switch {
		case src.DType == DTypeFloat32 && dt == DTypeFloat64:
			for i := 0; i < n; i++ {
				out.F64[i] = float64(src.F32[i])
			}
		case src.DType == DTypeFloat32 && dt == DTypeInt64:
			for i := 0; i < n; i++ {
				out.I64[i] = int64(src.F32[i])
			}
		case src.DType == DTypeFloat64 && dt == DTypeFloat32:
			for i := 0; i < n; i++ {
				out.F32[i] = float32(src.F64[i])
			}
		case src.DType == DTypeFloat64 && dt == DTypeInt64:
			for i := 0; i < n; i++ {
				out.I64[i] = int64(src.F64[i])
			}
		case src.DType == DTypeInt64 && dt == DTypeFloat32:
			for i := 0; i < n; i++ {
				out.F32[i] = float32(src.I64[i])
			}
		case src.DType == DTypeInt64 && dt == DTypeFloat64:
			for i := 0; i < n; i++ {
				out.F64[i] = float64(src.I64[i])
			}
		}
		return out
	}}
}

// ── Group 2: clip / linspace / arange / eye ──────────────────────────

func init() {
	// _tensor_clip(t: tensor, lo: number, hi: number) -> tensor
	//
	// Returns a new tensor with every element clamped to [lo, hi].
	// lo and hi must be numbers compatible with t's dtype (same rules
	// as _tensor_full: Float rejects i64; Integer widens to f32/f64).
	// lo <= hi is required; violating it errors cleanly.
	Builtins["_tensor_clip"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 3 {
			return runtimeError("_tensor_clip expects 3 arguments (tensor, lo, hi)", ast.Pos{})
		}
		t, ok := args[0].(*Tensor)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_clip: first argument must be a tensor, got %s", args[0].Type()), ast.Pos{})
		}
		if !tensorComputeAvailable() {
			return runtimeError("tensor ops require macOS or Linux in v1 (Windows support deferred)", ast.Pos{})
		}
		if !t.IsContiguous() {
			return runtimeError("_tensor_clip: non-contiguous inputs not yet supported", ast.Pos{})
		}
		out := newTensorFromShape(t.DType, t.Shape)
		switch t.DType {
		case DTypeFloat32:
			lo, errO := scalarToF32("_tensor_clip lo", args[1])
			if errO != nil {
				return errO
			}
			hi, errO := scalarToF32("_tensor_clip hi", args[2])
			if errO != nil {
				return errO
			}
			if lo > hi {
				return runtimeError(fmt.Sprintf("_tensor_clip: lo (%g) must be <= hi (%g)", lo, hi), ast.Pos{})
			}
			tensorClipF32(out.F32, t.F32, lo, hi)
		case DTypeFloat64:
			lo, errO := scalarToF64("_tensor_clip lo", args[1])
			if errO != nil {
				return errO
			}
			hi, errO := scalarToF64("_tensor_clip hi", args[2])
			if errO != nil {
				return errO
			}
			if lo > hi {
				return runtimeError(fmt.Sprintf("_tensor_clip: lo (%g) must be <= hi (%g)", lo, hi), ast.Pos{})
			}
			tensorClipF64(out.F64, t.F64, lo, hi)
		case DTypeInt64:
			lo, errO := scalarToI64("_tensor_clip lo", args[1])
			if errO != nil {
				return errO
			}
			hi, errO := scalarToI64("_tensor_clip hi", args[2])
			if errO != nil {
				return errO
			}
			if lo > hi {
				return runtimeError(fmt.Sprintf("_tensor_clip: lo (%d) must be <= hi (%d)", lo, hi), ast.Pos{})
			}
			tensorClipI64(out.I64, t.I64, lo, hi)
		}
		return out
	}}

	// _tensor_linspace(start: number, stop: number, n: int, dtype: string) -> tensor
	//
	// Returns a 1-D tensor of n evenly spaced values from start to stop
	// inclusive. n must be >= 1. For n == 1 the result is [start].
	// NumPy equivalent: np.linspace(start, stop, num=n).
	Builtins["_tensor_linspace"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 4 {
			return runtimeError("_tensor_linspace expects 4 arguments (start, stop, n, dtype)", ast.Pos{})
		}
		dtypeStr, ok := args[3].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_linspace: dtype must be a string, got %s", args[3].Type()), ast.Pos{})
		}
		dt, dok := dtypeFromName(dtypeStr.Value)
		if !dok {
			return typeError(fmt.Sprintf("_tensor_linspace: unknown dtype %q (want f32/f64/i64)", dtypeStr.Value), ast.Pos{})
		}
		nObj, ok := args[2].(*Integer)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_linspace: n must be an integer, got %s", args[2].Type()), ast.Pos{})
		}
		n := nObj.Value
		if n < 1 {
			return runtimeError(fmt.Sprintf("_tensor_linspace: n must be >= 1, got %d", n), ast.Pos{})
		}
		startF, errO := scalarToF64("_tensor_linspace start", args[0])
		if errO != nil {
			return errO
		}
		stopF, errO := scalarToF64("_tensor_linspace stop", args[1])
		if errO != nil {
			return errO
		}
		out := newTensorFromShape(dt, []int{n})
		step := 0.0
		if n > 1 {
			step = (stopF - startF) / float64(n-1)
		}
		switch dt {
		case DTypeFloat32:
			for i := 0; i < n; i++ {
				out.F32[i] = float32(startF + float64(i)*step)
			}
			out.F32[n-1] = float32(stopF) // pin endpoint exactly
		case DTypeFloat64:
			for i := 0; i < n; i++ {
				out.F64[i] = startF + float64(i)*step
			}
			out.F64[n-1] = stopF
		case DTypeInt64:
			for i := 0; i < n; i++ {
				out.I64[i] = int64(startF + float64(i)*step)
			}
		}
		return out
	}}

	// _tensor_arange(start: number, stop: number, step: number, dtype: string) -> tensor
	//
	// Returns a 1-D tensor of values from start up to (but not including)
	// stop, separated by step. step must be non-zero. NumPy equivalent:
	// np.arange(start, stop, step, dtype=dtype).
	Builtins["_tensor_arange"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 4 {
			return runtimeError("_tensor_arange expects 4 arguments (start, stop, step, dtype)", ast.Pos{})
		}
		dtypeStr, ok := args[3].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_arange: dtype must be a string, got %s", args[3].Type()), ast.Pos{})
		}
		dt, dok := dtypeFromName(dtypeStr.Value)
		if !dok {
			return typeError(fmt.Sprintf("_tensor_arange: unknown dtype %q (want f32/f64/i64)", dtypeStr.Value), ast.Pos{})
		}
		startF, errO := scalarToF64("_tensor_arange start", args[0])
		if errO != nil {
			return errO
		}
		stopF, errO := scalarToF64("_tensor_arange stop", args[1])
		if errO != nil {
			return errO
		}
		stepF, errO := scalarToF64("_tensor_arange step", args[2])
		if errO != nil {
			return errO
		}
		if stepF == 0 {
			return runtimeError("_tensor_arange: step must be non-zero", ast.Pos{})
		}
		// compute length: ceil((stop - start) / step), clamped to >= 0
		length := 0
		if stepF > 0 && stopF > startF {
			length = int((stopF-startF+stepF*1e-10)/stepF)
			if length < 0 {
				length = 0
			}
		} else if stepF < 0 && stopF < startF {
			length = int((startF-stopF-stepF*1e-10)/(-stepF))
			if length < 0 {
				length = 0
			}
		}
		out := newTensorFromShape(dt, []int{length})
		switch dt {
		case DTypeFloat32:
			for i := 0; i < length; i++ {
				out.F32[i] = float32(startF + float64(i)*stepF)
			}
		case DTypeFloat64:
			for i := 0; i < length; i++ {
				out.F64[i] = startF + float64(i)*stepF
			}
		case DTypeInt64:
			for i := 0; i < length; i++ {
				out.I64[i] = int64(startF + float64(i)*stepF)
			}
		}
		return out
	}}

	// _tensor_eye(n: int, dtype: string) -> tensor
	//
	// Returns an n×n identity matrix: 1 on the diagonal, 0 elsewhere.
	// NumPy equivalent: np.eye(n, dtype=dtype).
	Builtins["_tensor_eye"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_tensor_eye expects 2 arguments (n, dtype)", ast.Pos{})
		}
		nObj, ok := args[0].(*Integer)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_eye: n must be an integer, got %s", args[0].Type()), ast.Pos{})
		}
		dtypeStr, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_eye: dtype must be a string, got %s", args[1].Type()), ast.Pos{})
		}
		n := nObj.Value
		if n < 0 {
			return runtimeError(fmt.Sprintf("_tensor_eye: n must be non-negative, got %d", n), ast.Pos{})
		}
		dt, dok := dtypeFromName(dtypeStr.Value)
		if !dok {
			return typeError(fmt.Sprintf("_tensor_eye: unknown dtype %q (want f32/f64/i64)", dtypeStr.Value), ast.Pos{})
		}
		out := newTensorFromShape(dt, []int{n, n})
		switch dt {
		case DTypeFloat32:
			for i := 0; i < n; i++ {
				out.F32[i*n+i] = 1.0
			}
		case DTypeFloat64:
			for i := 0; i < n; i++ {
				out.F64[i*n+i] = 1.0
			}
		case DTypeInt64:
			for i := 0; i < n; i++ {
				out.I64[i*n+i] = 1
			}
		}
		return out
	}}
}

// ── Group 3: comparison ops + where ─────────────────────────────────

// comparisonKernel bundles the three dtype-specific kernel pointers for
// one element-wise comparison op. Output is always i64 (0 or 1).
type comparisonKernel struct {
	f32 func(out []int64, a, b []float32)
	f64 func(out []int64, a, b []float64)
	i64 func(out, a, b []int64)
}

// elementWiseComparison is the shared dispatch helper for all six
// comparison builtins (eq / ne / lt / le / gt / ge). It follows the
// same broadcasting and scalar-promotion rules as elementWiseBinary
// but always produces an i64 output tensor (mask of 0/1).
func elementWiseComparison(args []Object, opName string, k comparisonKernel) Object {
	if len(args) != 2 {
		return runtimeError(fmt.Sprintf("%s expects 2 arguments (a, b)", opName), ast.Pos{})
	}
	if !tensorComputeAvailable() {
		return runtimeError("tensor ops require macOS or Linux in v1 (Windows support deferred)", ast.Pos{})
	}
	aTen, aIsT := args[0].(*Tensor)
	bTen, bIsT := args[1].(*Tensor)
	if !aIsT && !bIsT {
		return typeError(fmt.Sprintf("%s: at least one argument must be a tensor (got %s and %s)", opName, args[0].Type(), args[1].Type()), ast.Pos{})
	}
	var ref *Tensor
	if aIsT {
		ref = aTen
	} else {
		ref = bTen
	}
	if !aIsT {
		mat, errObj := tensorFromScalar(args[0], ref)
		if errObj != nil {
			return errObj
		}
		aTen = mat
	}
	if !bIsT {
		mat, errObj := tensorFromScalar(args[1], ref)
		if errObj != nil {
			return errObj
		}
		bTen = mat
	}
	if aTen.DType != bTen.DType {
		return typeError(fmt.Sprintf("%s: dtype mismatch (%s vs %s); explicit conversion required", opName, aTen.DType, bTen.DType), ast.Pos{})
	}
	if !aTen.IsContiguous() || !bTen.IsContiguous() {
		return runtimeError(fmt.Sprintf("%s: non-contiguous inputs not yet supported", opName), ast.Pos{})
	}
	outShape, bcErr := broadcastShape(aTen.Shape, bTen.Shape)
	if bcErr != "" {
		return runtimeError(fmt.Sprintf("%s: %s", opName, bcErr), ast.Pos{})
	}
	if !sameShape(aTen.Shape, outShape) {
		aTen = materializeBroadcast(aTen, outShape)
	}
	if !sameShape(bTen.Shape, outShape) {
		bTen = materializeBroadcast(bTen, outShape)
	}
	out := newTensorFromShape(DTypeInt64, outShape)
	switch aTen.DType {
	case DTypeFloat32:
		k.f32(out.I64, aTen.F32, bTen.F32)
	case DTypeFloat64:
		k.f64(out.I64, aTen.F64, bTen.F64)
	case DTypeInt64:
		k.i64(out.I64, aTen.I64, bTen.I64)
	}
	return out
}

func init() {
	// _tensor_eq(a, b) -> tensor
	// Element-wise equality: out[i] = 1 if a[i] == b[i] else 0. Output is
	// always i64 regardless of input dtype. Operands may be tensor+tensor
	// (broadcast-compatible) or tensor+scalar. NaN follows IEEE 754: any
	// comparison with NaN is 0 (false). Pair with _tensor_where for
	// mask-based selection. NumPy parallel: np.equal(a, b) cast to int64.
	Builtins["_tensor_eq"] = &Builtin{Fn: func(args []Object) Object {
		return elementWiseComparison(args, "_tensor_eq", comparisonKernel{f32: tensorEqF32, f64: tensorEqF64, i64: tensorEqI64})
	}}
	// _tensor_ne(a, b) -> tensor
	// Element-wise inequality: out[i] = 1 if a[i] != b[i] else 0. Output is
	// i64. NaN comparisons: NaN != NaN is 1 (matches IEEE 754 and NumPy).
	// Same broadcasting + scalar rules as _tensor_eq.
	Builtins["_tensor_ne"] = &Builtin{Fn: func(args []Object) Object {
		return elementWiseComparison(args, "_tensor_ne", comparisonKernel{f32: tensorNeF32, f64: tensorNeF64, i64: tensorNeI64})
	}}
	// _tensor_lt(a, b) -> tensor
	// Element-wise less-than: out[i] = 1 if a[i] < b[i] else 0. Output is
	// i64. NaN comparisons return 0. Same broadcasting + scalar rules as
	// _tensor_eq.
	Builtins["_tensor_lt"] = &Builtin{Fn: func(args []Object) Object {
		return elementWiseComparison(args, "_tensor_lt", comparisonKernel{f32: tensorLtF32, f64: tensorLtF64, i64: tensorLtI64})
	}}
	// _tensor_le(a, b) -> tensor
	// Element-wise less-than-or-equal: out[i] = 1 if a[i] <= b[i] else 0.
	// Output is i64. NaN comparisons return 0. Same broadcasting + scalar
	// rules as _tensor_eq.
	Builtins["_tensor_le"] = &Builtin{Fn: func(args []Object) Object {
		return elementWiseComparison(args, "_tensor_le", comparisonKernel{f32: tensorLeF32, f64: tensorLeF64, i64: tensorLeI64})
	}}
	// _tensor_gt(a, b) -> tensor
	// Element-wise greater-than: out[i] = 1 if a[i] > b[i] else 0. Output
	// is i64. NaN comparisons return 0. Same broadcasting + scalar rules
	// as _tensor_eq.
	Builtins["_tensor_gt"] = &Builtin{Fn: func(args []Object) Object {
		return elementWiseComparison(args, "_tensor_gt", comparisonKernel{f32: tensorGtF32, f64: tensorGtF64, i64: tensorGtI64})
	}}
	// _tensor_ge(a, b) -> tensor
	// Element-wise greater-than-or-equal: out[i] = 1 if a[i] >= b[i] else 0.
	// Output is i64. NaN comparisons return 0. Same broadcasting + scalar
	// rules as _tensor_eq.
	Builtins["_tensor_ge"] = &Builtin{Fn: func(args []Object) Object {
		return elementWiseComparison(args, "_tensor_ge", comparisonKernel{f32: tensorGeF32, f64: tensorGeF64, i64: tensorGeI64})
	}}

	// _tensor_where(mask: tensor, x: tensor|number, y: tensor|number) -> tensor
	//
	// Element-wise conditional selection: out[i] = x[i] if mask[i] != 0 else y[i].
	// mask must be an i64 tensor (typically the output of a comparison op).
	// x and y must have the same dtype; either may be a scalar number (broadcast
	// to mask's shape). All shapes must match mask's shape after scalar promotion.
	// NumPy equivalent: np.where(condition, x, y).
	Builtins["_tensor_where"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 3 {
			return runtimeError("_tensor_where expects 3 arguments (mask, x, y)", ast.Pos{})
		}
		mask, ok := args[0].(*Tensor)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_where: mask must be a tensor, got %s", args[0].Type()), ast.Pos{})
		}
		if mask.DType != DTypeInt64 {
			return typeError(fmt.Sprintf("_tensor_where: mask must be an i64 tensor (got %s); use a comparison op to produce the mask", mask.DType), ast.Pos{})
		}
		if !mask.IsContiguous() {
			return runtimeError("_tensor_where: non-contiguous mask not yet supported", ast.Pos{})
		}
		// Determine reference dtype from whichever of x/y is a tensor.
		var xTen, yTen *Tensor
		var refDType DType
		if xt, ok := args[1].(*Tensor); ok {
			xTen = xt
			refDType = xt.DType
		}
		if yt, ok := args[2].(*Tensor); ok {
			yTen = yt
			if xTen != nil && xTen.DType != yt.DType {
				return typeError(fmt.Sprintf("_tensor_where: x dtype (%s) != y dtype (%s)", xTen.DType, yt.DType), ast.Pos{})
			}
			refDType = yt.DType
		}
		if xTen == nil && yTen == nil {
			return typeError("_tensor_where: at least one of x or y must be a tensor", ast.Pos{})
		}
		// promote scalars
		refShape := mask.Shape
		if xTen == nil {
			refT := &Tensor{DType: refDType, Shape: refShape}
			switch refDType {
			case DTypeFloat32:
				refT.F32 = make([]float32, mask.Numel())
			case DTypeFloat64:
				refT.F64 = make([]float64, mask.Numel())
			case DTypeInt64:
				refT.I64 = make([]int64, mask.Numel())
			}
			mat, errObj := tensorFromScalar(args[1], refT)
			if errObj != nil {
				return errObj
			}
			xTen = mat
		}
		if yTen == nil {
			refT := &Tensor{DType: refDType, Shape: refShape}
			switch refDType {
			case DTypeFloat32:
				refT.F32 = make([]float32, mask.Numel())
			case DTypeFloat64:
				refT.F64 = make([]float64, mask.Numel())
			case DTypeInt64:
				refT.I64 = make([]int64, mask.Numel())
			}
			mat, errObj := tensorFromScalar(args[2], refT)
			if errObj != nil {
				return errObj
			}
			yTen = mat
		}
		if !sameShape(mask.Shape, xTen.Shape) || !sameShape(mask.Shape, yTen.Shape) {
			return runtimeError(fmt.Sprintf("_tensor_where: mask shape %v, x shape %v, y shape %v must all match (broadcasting across mask not yet supported)", mask.Shape, xTen.Shape, yTen.Shape), ast.Pos{})
		}
		if !xTen.IsContiguous() || !yTen.IsContiguous() {
			return runtimeError("_tensor_where: non-contiguous x or y not yet supported", ast.Pos{})
		}
		n := mask.Numel()
		out := newTensorFromShape(refDType, mask.Shape)
		switch refDType {
		case DTypeFloat32:
			for i := 0; i < n; i++ {
				if mask.I64[i] != 0 {
					out.F32[i] = xTen.F32[i]
				} else {
					out.F32[i] = yTen.F32[i]
				}
			}
		case DTypeFloat64:
			for i := 0; i < n; i++ {
				if mask.I64[i] != 0 {
					out.F64[i] = xTen.F64[i]
				} else {
					out.F64[i] = yTen.F64[i]
				}
			}
		case DTypeInt64:
			for i := 0; i < n; i++ {
				if mask.I64[i] != 0 {
					out.I64[i] = xTen.I64[i]
				} else {
					out.I64[i] = yTen.I64[i]
				}
			}
		}
		return out
	}}
}

// ── Group 4: concatenate / stack ─────────────────────────────────────

// tensorConcatenateGo joins a slice of validated same-dtype tensors along
// axis. All validation (rank, shape compatibility, dtype match) is done
// by the _tensor_concatenate builtin before calling this. The helper uses
// a prefix/suffix copy strategy so each copy is a single contiguous block:
//
//	prefix = product(shape[0 .. axis-1])   — number of outer blocks
//	suffix = product(shape[axis+1 .. R-1]) — elements per axis slice
//
// For each outer block, copy axisSize*suffix elements from each tensor
// in sequence. This runs at near-memcpy speed.
func tensorConcatenateGo(tensors []*Tensor, axis int) *Tensor {
	dtype := tensors[0].DType
	rank := len(tensors[0].Shape)
	outShape := make([]int, rank)
	copy(outShape, tensors[0].Shape)
	for i := 1; i < len(tensors); i++ {
		outShape[axis] += tensors[i].Shape[axis]
	}
	out := newTensorFromShape(dtype, outShape)
	prefix := 1
	for i := 0; i < axis; i++ {
		prefix *= outShape[i]
	}
	suffix := 1
	for i := axis + 1; i < rank; i++ {
		suffix *= outShape[i]
	}
	dstAxisOff := 0
	for _, t := range tensors {
		axisSize := t.Shape[axis]
		blockLen := axisSize * suffix
		for p := 0; p < prefix; p++ {
			srcOff := p * axisSize * suffix
			dstOff := p*outShape[axis]*suffix + dstAxisOff*suffix
			switch dtype {
			case DTypeFloat32:
				copy(out.F32[dstOff:dstOff+blockLen], t.F32[srcOff:srcOff+blockLen])
			case DTypeFloat64:
				copy(out.F64[dstOff:dstOff+blockLen], t.F64[srcOff:srcOff+blockLen])
			case DTypeInt64:
				copy(out.I64[dstOff:dstOff+blockLen], t.I64[srcOff:srcOff+blockLen])
			}
		}
		dstAxisOff += axisSize
	}
	return out
}

func init() {
	// _tensor_concatenate(tensors: array, axis: int) -> tensor
	//
	// Joins an array of tensors along an existing axis. All tensors must:
	//   - have the same dtype
	//   - have the same rank
	//   - have identical shapes on every axis except `axis`
	// NumPy equivalent: np.concatenate([t1, t2, ...], axis=N).
	Builtins["_tensor_concatenate"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_tensor_concatenate expects 2 arguments (tensors, axis)", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_concatenate: first argument must be an array of tensors, got %s", args[0].Type()), ast.Pos{})
		}
		axisObj, ok := args[1].(*Integer)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_concatenate: axis must be an integer, got %s", args[1].Type()), ast.Pos{})
		}
		if len(arr.Elements) == 0 {
			return runtimeError("_tensor_concatenate: tensors array must not be empty", ast.Pos{})
		}
		tensors := make([]*Tensor, len(arr.Elements))
		for i, el := range arr.Elements {
			t, ok := el.(*Tensor)
			if !ok {
				return typeError(fmt.Sprintf("_tensor_concatenate: element %d must be a tensor, got %s", i, el.Type()), ast.Pos{})
			}
			if !t.IsContiguous() {
				return runtimeError(fmt.Sprintf("_tensor_concatenate: tensor at index %d is non-contiguous (not yet supported)", i), ast.Pos{})
			}
			tensors[i] = t
		}
		ref := tensors[0]
		rank := len(ref.Shape)
		axis := axisObj.Value
		if axis < 0 {
			axis = axis + rank
		}
		if axis < 0 || axis >= rank {
			return runtimeError(fmt.Sprintf("_tensor_concatenate: axis %d out of range for rank-%d tensors", axisObj.Value, rank), ast.Pos{})
		}
		for i := 1; i < len(tensors); i++ {
			t := tensors[i]
			if t.DType != ref.DType {
				return typeError(fmt.Sprintf("_tensor_concatenate: dtype mismatch at index %d (%s vs %s)", i, t.DType, ref.DType), ast.Pos{})
			}
			if len(t.Shape) != rank {
				return runtimeError(fmt.Sprintf("_tensor_concatenate: rank mismatch at index %d (%d vs %d)", i, len(t.Shape), rank), ast.Pos{})
			}
			for d := 0; d < rank; d++ {
				if d != axis && t.Shape[d] != ref.Shape[d] {
					return runtimeError(fmt.Sprintf("_tensor_concatenate: shape mismatch at index %d dimension %d (%d vs %d)", i, d, t.Shape[d], ref.Shape[d]), ast.Pos{})
				}
			}
		}
		return tensorConcatenateGo(tensors, axis)
	}}

	// _tensor_stack(tensors: array, axis: int) -> tensor
	//
	// Joins an array of tensors along a NEW axis. All tensors must have
	// identical shapes. The output has rank = input_rank + 1, with the
	// new axis of size len(tensors) inserted at position `axis`.
	// NumPy equivalent: np.stack([t1, t2, ...], axis=N).
	Builtins["_tensor_stack"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_tensor_stack expects 2 arguments (tensors, axis)", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_stack: first argument must be an array of tensors, got %s", args[0].Type()), ast.Pos{})
		}
		axisObj, ok := args[1].(*Integer)
		if !ok {
			return typeError(fmt.Sprintf("_tensor_stack: axis must be an integer, got %s", args[1].Type()), ast.Pos{})
		}
		if len(arr.Elements) == 0 {
			return runtimeError("_tensor_stack: tensors array must not be empty", ast.Pos{})
		}
		tensors := make([]*Tensor, len(arr.Elements))
		for i, el := range arr.Elements {
			t, ok := el.(*Tensor)
			if !ok {
				return typeError(fmt.Sprintf("_tensor_stack: element %d must be a tensor, got %s", i, el.Type()), ast.Pos{})
			}
			if !t.IsContiguous() {
				return runtimeError(fmt.Sprintf("_tensor_stack: tensor at index %d is non-contiguous (not yet supported)", i), ast.Pos{})
			}
			tensors[i] = t
		}
		ref := tensors[0]
		rank := len(ref.Shape)
		axis := axisObj.Value
		if axis < 0 {
			axis = axis + rank + 1
		}
		if axis < 0 || axis > rank {
			return runtimeError(fmt.Sprintf("_tensor_stack: axis %d out of range for rank-%d tensors (valid: -%d to %d)", axisObj.Value, rank, rank+1, rank), ast.Pos{})
		}
		for i := 1; i < len(tensors); i++ {
			t := tensors[i]
			if t.DType != ref.DType {
				return typeError(fmt.Sprintf("_tensor_stack: dtype mismatch at index %d (%s vs %s)", i, t.DType, ref.DType), ast.Pos{})
			}
			if !sameShape(t.Shape, ref.Shape) {
				return runtimeError(fmt.Sprintf("_tensor_stack: shape mismatch at index %d (%v vs %v); all tensors must have identical shapes", i, t.Shape, ref.Shape), ast.Pos{})
			}
		}
		// expand each tensor at axis to add the new dimension, then concatenate
		expanded := make([]*Tensor, len(tensors))
		for i, t := range tensors {
			newShape := make([]int, rank+1)
			copy(newShape[:axis], t.Shape[:axis])
			newShape[axis] = 1
			copy(newShape[axis+1:], t.Shape[axis:])
			expanded[i] = &Tensor{DType: t.DType, Shape: newShape, F32: t.F32, F64: t.F64, I64: t.I64}
		}
		return tensorConcatenateGo(expanded, axis)
	}}
}

// ── scalar extraction helpers (used by clip, linspace, arange) ───────

// scalarToF32 extracts a float32 from a kLex Integer or Float.
// opName is used in the error message. Returns (0, *Error) on failure.
func scalarToF32(opName string, v Object) (float32, Object) {
	switch x := v.(type) {
	case *Integer:
		return float32(x.Value), nil
	case *Float:
		return float32(x.Value), nil
	}
	return 0, typeError(fmt.Sprintf("%s: expected a number, got %s", opName, v.Type()), ast.Pos{})
}

// scalarToF64 extracts a float64 from a kLex Integer or Float.
func scalarToF64(opName string, v Object) (float64, Object) {
	switch x := v.(type) {
	case *Integer:
		return float64(x.Value), nil
	case *Float:
		return x.Value, nil
	}
	return 0, typeError(fmt.Sprintf("%s: expected a number, got %s", opName, v.Type()), ast.Pos{})
}

// scalarToI64 extracts an int64 from a kLex Integer. Rejects Float
// (consistent with kLex's strict i64 policy — convert explicitly).
func scalarToI64(opName string, v Object) (int64, Object) {
	switch x := v.(type) {
	case *Integer:
		return int64(x.Value), nil
	case *Float:
		return 0, typeError(fmt.Sprintf("%s: cannot use a float for an i64 tensor parameter (use an integer)", opName), ast.Pos{})
	}
	return 0, typeError(fmt.Sprintf("%s: expected an integer, got %s", opName, v.Type()), ast.Pos{})
}

// parseSliceSpec parses one axis's spec for _tensor_slice. Returns
// (start, stop, step) or an *Error.
//
// Accepts:
//
//	null            — equivalent to [null, null, null] (full axis)
//	[a, b, c]       — start=a, stop=b, step=c; any element may be
//	                  null to take its default (0 / dim / 1)
//
// Start/stop are returned WITHOUT negative-index normalisation —
// the caller adjusts after parsing, since axis bounds-clamping is
// also a caller responsibility.
//
// step must be >= 1 (negative-step slicing deferred).
func parseSliceSpec(spec Object, dim, axis int) (int, int, int, Object) {
	if spec == NULL {
		return 0, dim, 1, nil
	}
	arr, ok := spec.(*Array)
	if !ok {
		return 0, 0, 0, typeError(fmt.Sprintf("_tensor_slice: spec for axis %d must be null or [start, stop, step], got %s", axis, spec.Type()), ast.Pos{})
	}
	if len(arr.Elements) != 3 {
		return 0, 0, 0, runtimeError(fmt.Sprintf("_tensor_slice: spec for axis %d must be a 3-element [start, stop, step], got length %d", axis, len(arr.Elements)), ast.Pos{})
	}
	start := 0
	if arr.Elements[0] != NULL {
		si, ok := arr.Elements[0].(*Integer)
		if !ok {
			return 0, 0, 0, typeError(fmt.Sprintf("_tensor_slice: axis %d start must be integer or null, got %s", axis, arr.Elements[0].Type()), ast.Pos{})
		}
		start = si.Value
	}
	stop := dim
	if arr.Elements[1] != NULL {
		si, ok := arr.Elements[1].(*Integer)
		if !ok {
			return 0, 0, 0, typeError(fmt.Sprintf("_tensor_slice: axis %d stop must be integer or null, got %s", axis, arr.Elements[1].Type()), ast.Pos{})
		}
		stop = si.Value
	}
	step := 1
	if arr.Elements[2] != NULL {
		si, ok := arr.Elements[2].(*Integer)
		if !ok {
			return 0, 0, 0, typeError(fmt.Sprintf("_tensor_slice: axis %d step must be integer or null, got %s", axis, arr.Elements[2].Type()), ast.Pos{})
		}
		step = si.Value
	}
	if step <= 0 {
		return 0, 0, 0, runtimeError(fmt.Sprintf("_tensor_slice: axis %d step must be positive (negative-step slicing not yet supported), got %d", axis, step), ast.Pos{})
	}
	return start, stop, step, nil
}
