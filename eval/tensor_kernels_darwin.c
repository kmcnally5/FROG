/* tensor_kernels_darwin.c — darwin-only shim that pulls in the shared
 * kernel source. The actual implementations live in tensor_kernels.inc;
 * this file exists purely so Go's filename build-tag rules
 * (_darwin.c → darwin-only) gate the C compile to darwin builds.
 *
 * Why: Go's package scanner objects to a bare .c file in a package
 * where no in-scope Go file imports "C". On Windows the cgo file is
 * build-tagged out, so a darwin/linux-shared tensor_kernels.c sitting
 * here triggers the error
 *   "C source files not allowed when not using cgo or SWIG".
 * Splitting by OS-suffixed shims keeps Windows clean.
 */
#include "tensor_kernels.inc"
