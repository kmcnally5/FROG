// main.lex
import "jpeg.lex" as jpg

fn worker(id, blocks, bus) {
    let compressed = []
    for b in blocks {
        compressed = push(compressed, jpg.compress_block(b))
    }
    send(bus, { "worker": id, "data": compressed })
}

fn main() {
    let img_width = 64
    let img_height = 64
    let blocks_count = (img_width * img_height) / 64
    
    let bus = channel(8)
    println("--- kLex Parallel JPEG Engine ---")
    
    // Simulate dividing an image into 8 chunks of blocks
    for i in range(8) {
        let chunk = [] // Populate with 8x8 arrays of raw Y pixels
        async(worker, i, chunk, bus)
    }
    
    // Collect compressed macroblocks
    let finished = 0
    while finished < 8 {
        res, ok = recv(bus)
        if ok { 
            finished = finished + 1 
            println("Block chunk " + str(res.worker) + " compressed.")
        }
    }
    println("Image Compression Pipeline Complete.")
}