package spa

// ArgumentProbeExpr asks a page function WHAT IT READS from its arguments.
//
// WHY IT EXISTS. Reading a function's shape has three techniques in this module,
// and ARMADILHAS.md records where each one stops:
//
//	String(fn)                works only when the function is synchronous;
//	                          an async one returns apply(this, arguments)
//	the app's own call site    works only when there IS one, and only when you
//	                          verify it calls the function you mean (H69)
//	the model layer above     works only when a model exposes the operation
//
// Poll creation (H69) and label association (H72) are blocked because all three
// fail at once: async wrapper, no call site in the shipped bundles, no model
// method. That is not one missing signature — it is a missing INSTRUMENT, which
// is why this is a general expression and not a poll-specific script.
//
// HOW IT WORKS. A function that destructures an object must READ its
// properties, and reading is observable: it is handed a Proxy whose get trap
// records every key. The function then usually throws — undefined is rarely a
// valid value for anything — and by then it has already said what it wanted.
//
// It answers "which fields", never "which values", so it cannot tell apart two
// fields the function reads under a condition it never reaches. A second pass
// with values supplied for the first pass's keys goes deeper; the caller decides
// whether that is worth it.
//
// WHAT IT IS NOT. It does not intercept a real call, so it does not reveal what
// the APP would pass — only what the function LOOKS FOR. Those are different
// questions, and confusing them is how H69 went wrong in the other direction.
//
// DEPTH. maxDepth 1 reports the top-level fields; deeper reports PATHS like
// "poll.filteredOptions", because the recorder answers a read with another
// recorder instead of undefined. Depth costs nothing but noise: a function that
// walks a long chain reports the whole chain.
//
// SHAPES. Per-argument hints — "object" (default), "array", "string", "number".
// "array" hands a REAL array holding one recorder, which is the only way to see
// what an iterating function reads off an element: a recorder answers forEach
// itself and the callback never runs.
//
// It is a JavaScript function of (moduleName, fnName, arity, maxDepth, shapes)
// returning {ok, why, reads: [[path, ...], ...], threw}.
const ArgumentProbeExpr = `(async function (moduleName, fnName, arity, maxDepth, shapes) {
	let mod, fn;
	try {
		mod = window.require(moduleName);
	} catch (e) {
		return { ok: false, why: 'MODULE_THREW: ' + String((e && e.message) || e).slice(0, 120) };
	}
	if (!mod) { return { ok: false, why: 'NO_MODULE' }; }
	fn = mod[fnName];
	if (typeof fn !== 'function') {
		return { ok: false, why: 'NOT_A_FUNCTION: ' + typeof fn };
	}

	const depth = (typeof maxDepth === 'number' && maxDepth > 0) ? maxDepth : 1;
	const reads = [];

	// recorder builds a proxy that records the PATH of every property read.
	// Below maxDepth it answers with another recorder, so a function that
	// destructures an envelope and then reaches inside one of its fields —
	// which is what a poll payload does — reports poll.filteredOptions rather
	// than stopping at poll.
	//
	// THE TARGET IS A FUNCTION on purpose: some call paths test their argument
	// with typeof or call it, and a proxy over {} answers 'object' and cannot be
	// called, which ends the read early with a TypeError that says nothing.
	const recorder = (seen, path, level) => new Proxy(function () {}, {
		get(target, key) {
			// Symbols are the runtime asking structural questions —
			// Symbol.iterator, Symbol.toPrimitive — not the function asking for
			// a field. Recording them would bury the answer in noise.
			if (typeof key === 'symbol') { return undefined; }
			// 'then' MUST be undefined: an awaited proxy whose then is callable
			// never settles, and the probe would hang instead of answering.
			if (key === 'then') { return undefined; }
			// babelHelpers.extends and Object.assign read these on every object
			// they touch; they say nothing about the contract.
			if (key === 'hasOwnProperty' || key === 'constructor' ||
			    key === 'prototype' || key === 'toJSON') { return undefined; }
			const full = path ? (path + '.' + key) : key;
			if (seen.indexOf(full) === -1) { seen.push(full); }
			return level < depth ? recorder(seen, full, level + 1) : undefined;
		},
		apply() { return level < depth ? recorder(seen, path + '()', level + 1) : undefined; },
		// A destructuring assignment with defaults consults ownKeys on some
		// engines; answering with what has been asked for so far keeps the
		// proxy from looking empty and short-circuiting the read.
		ownKeys() { return []; },
		getOwnPropertyDescriptor() {
			return { configurable: true, enumerable: true, value: undefined };
		}
	});

	// SHAPE HINTS. A function that iterates an argument — forEach, map — never
	// hands the recorder to its callback, because the recorder answers the
	// iteration itself and the callback is never called. Passing a REAL array
	// holding one recorder fixes that: the iteration is real, and the element
	// reports what the callback reads off it.
	//
	// This was not foresight. The label bridge answered ".forEach" and nothing
	// more, and the reason was structural rather than specific to labels — any
	// API taking a list has it.
	const args = [];
	for (let i = 0; i < arity; i++) {
		const seen = [];
		reads.push(seen);
		const hint = (shapes && shapes[i]) || 'object';
		if (hint === 'array') {
			args.push([recorder(seen, '[0]', 1)]);
		} else if (hint === 'string') {
			// Some functions take primitives and read nothing; handing them a
			// proxy only produces a type error that says less than a real value.
			args.push('wa-headless-probe');
		} else if (hint === 'number') {
			args.push(0);
		} else {
			args.push(recorder(seen, '', 1));
		}
	}

	let threw = '';
	try {
		await fn.apply(mod, args);
	} catch (e) {
		threw = String((e && e.message) || e).slice(0, 200);
	}
	return { ok: true, why: '', reads: reads, threw: threw };
})`
