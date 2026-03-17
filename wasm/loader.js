// gotreesitter WASM loader
// Usage:
//   <script src="wasm_exec.js"></script>
//   <script src="loader.js"></script>
//   <script>
//     loadGotreesitter("gotreesitter.wasm").then(() => {
//       // Import a grammar from JSON
//       const result = gotreesitter.importGrammar(grammarJSON);
//       if (!result.ok) throw new Error(result.error);
//
//       // Generate the parser
//       gotreesitter.generateLanguage(result.name);
//
//       // Highlight source code
//       const hl = gotreesitter.highlight(result.name, sourceCode, highlightQuery);
//       // hl.ranges = [{startByte, endByte, capture}, ...]
//     });
//   </script>

async function loadGotreesitter(wasmPath) {
  const go = new Go();
  let result;
  if (typeof WebAssembly.instantiateStreaming === "function") {
    result = await WebAssembly.instantiateStreaming(fetch(wasmPath), go.importObject);
  } else {
    const resp = await fetch(wasmPath);
    const bytes = await resp.arrayBuffer();
    result = await WebAssembly.instantiate(bytes, go.importObject);
  }
  go.run(result.instance);
  // gotreesitter is now available as a global
  return window.gotreesitter;
}
