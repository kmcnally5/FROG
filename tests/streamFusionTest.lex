import "stream_fusion.lex" as sf

arr = [1, 2, 3, 4, 5, 6]

res = sf.fuse(
    arr,
    sf.mapStep(fn(x) { x * 2 }),
    sf.filterStep(fn(x) { x % 2 == 0 })
)

println(res[0])
println(res[1])
println(len(res))
