import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const source = readFileSync(
  new URL("./DetailsEditNavbar.tsx", import.meta.url),
  "utf8"
);

test("delete contract is optional and both delete surfaces fail closed", () => {
  assert.match(source, /onDelete\?: \(\) => void;/);
  assert.match(
    source,
    /if \(props\.isNew \|\| props\.isEditing \|\| !props\.onDelete\) return;/
  );
  assert.match(
    source,
    /function renderDeleteAlert\(\) \{\s+if \(!props\.onDelete\) return;/
  );
});

test("custom button contract accepts React null without widening browser globals", () => {
  assert.match(source, /customButtons\?: React\.ReactNode;/);
});
