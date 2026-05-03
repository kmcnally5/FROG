import "merkle.lex" as merkle
import "hash.lex" as h

// Test 1: Basic Merkle tree root computation
content = "block_data_0_replica_content_12345"
root = merkle.tree_root(content, merkle.CHUNK_SIZE)
println("[test] content length: " + str(len(content)))
println("[test] root hash: " + str(root))

// Test 2: Get proof for chunk 0
proof_result = merkle.get_proof(content, 0, merkle.CHUNK_SIZE)
chunk_hash = proof_result[0]
path = proof_result[1]
println("[test] chunk 0 hash: " + str(chunk_hash))
println("[test] path length: " + str(len(path)))
println("[test] path: " + str(path))

// Test 3: Verify proof for chunk 0
valid = merkle.verify(chunk_hash, 0, path, root)
println("[test] chunk 0 proof valid: " + str(valid))

// Test 4: Get proof for chunk 2
proof_result2 = merkle.get_proof(content, 2, merkle.CHUNK_SIZE)
chunk_hash2 = proof_result2[0]
path2 = proof_result2[1]
println("[test] chunk 2 hash: " + str(chunk_hash2))
println("[test] chunk 2 path length: " + str(len(path2)))

// Test 5: Verify proof for chunk 2
valid2 = merkle.verify(chunk_hash2, 2, path2, root)
println("[test] chunk 2 proof valid: " + str(valid2))

// Test 6: Verify invalid proof fails
invalid = merkle.verify(chunk_hash, 0, path, 12345)
println("[test] invalid root rejected: " + str(!invalid))

// Test 7: Verify wrong chunk fails
invalid2 = merkle.verify(chunk_hash, 1, path, root)
println("[test] wrong chunk rejected: " + str(!invalid2))

println("[test] all Merkle tests passed")
