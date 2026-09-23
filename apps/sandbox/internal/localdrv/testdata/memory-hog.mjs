// Fills real pages rather than reserving address space, so a memory ceiling
// has something to act on.
console.log("the hog is awake");
const held = [];
for (let i = 0; i < 256; i++) {
  held.push(Buffer.alloc(8 << 20, i % 256));
}
console.log("the hog allocated 2 GiB unhindered, total=" + held.length);
