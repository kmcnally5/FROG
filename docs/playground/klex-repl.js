let inputBuffer = '';

const inputField = document.getElementById('input');
const runBtn = document.getElementById('run-btn');
const outputDiv = document.getElementById('output');
const resetBtn = document.getElementById('reset-btn');
const promptSpan = document.getElementById('prompt');

function addOutput(text, isError = false) {
	const line = document.createElement('div');
	line.className = isError ? 'line error' : 'line';
	line.textContent = text;
	outputDiv.appendChild(line);
	outputDiv.scrollTop = outputDiv.scrollHeight;
}

function setPrompt(continuation) {
	promptSpan.textContent = continuation ? '.. ' : '>> ';
}

function submit() {
	const line = inputField.value;
	inputField.value = '';

	if (line.trim() === 'exit') {
		addOutput('bye');
		inputField.disabled = true;
		runBtn.disabled = true;
		return;
	}

	const isFirstLine = inputBuffer === '';
	inputBuffer = inputBuffer ? inputBuffer + '\n' + line : line;

	// Check brace depth — if open, show the line and wait for more input
	const d = window.klex_depth ? window.klex_depth(inputBuffer) : 0;
	if (d > 0) {
		addOutput((isFirstLine ? '>> ' : '.. ') + line);
		setPrompt(true);
		return;
	}

	// Balanced — show closing line then evaluate
	addOutput((isFirstLine ? '>> ' : '.. ') + line);

	const src = inputBuffer.trim();
	inputBuffer = '';
	setPrompt(false);

	if (src === '') return;

	if (window.klex_eval) {
		try {
			const result = window.klex_eval(src);
			if (result.output) {
				result.output.split('\n').forEach(l => { if (l) addOutput(l); });
			}
			if (result.error) {
				result.error.split('\n').forEach(l => { if (l) addOutput(l, true); });
			}
		} catch (err) {
			addOutput('Error: ' + err.message, true);
		}
	}
}

runBtn.onclick = submit;

resetBtn.onclick = function() {
	outputDiv.innerHTML = '';
	inputBuffer = '';
	inputField.value = '';
	inputField.disabled = false;
	runBtn.disabled = false;
	setPrompt(false);
	if (window.klex_reset) window.klex_reset();
	addOutput('kLex REPL — type "exit" to quit');
};

inputField.onkeypress = function(e) {
	if (e.key === 'Enter') {
		e.preventDefault();
		submit();
	}
};

async function initWasm() {
	try {
		const go = new Go();
		const response = await fetch('klex.wasm');
		const buffer = await response.arrayBuffer();
		const result = await WebAssembly.instantiate(buffer, go.importObject);
		go.run(result.instance);
		addOutput('kLex REPL — type "exit" to quit');
	} catch (err) {
		console.error('WASM load error:', err);
		addOutput('Error loading WASM: ' + err.message, true);
	}
}

window.addEventListener('load', initWasm);
