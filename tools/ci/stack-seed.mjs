const CRC_TABLE = (() => {
  const t = new Int32Array(256);
  for (let n = 0; n < 256; n++) {
    let c = n;
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    t[n] = c;
  }
  return t;
})();

function crc32(buf) {
  let c = 0 ^ -1;
  for (const b of buf) c = (c >>> 8) ^ CRC_TABLE[(c ^ b) & 0xff];
  return (c ^ -1) >>> 0;
}

// Builds a minimal store-only (uncompressed) zip archive by hand: a local
// file header, the file bytes, a central directory entry, then the
// end-of-central-directory record.
export function zipOneFile(name, text) {
  const nameBytes = Buffer.from(name, "utf8");
  const data = Buffer.from(text, "utf8");
  const sum = crc32(data);

  const local = Buffer.alloc(30);
  local.writeUInt32LE(0x04034b50, 0);
  local.writeUInt16LE(20, 4);
  local.writeUInt16LE(0, 6);
  local.writeUInt16LE(0, 8);
  local.writeUInt16LE(0, 10);
  local.writeUInt16LE(0x21, 12);
  local.writeUInt32LE(sum, 14);
  local.writeUInt32LE(data.length, 18);
  local.writeUInt32LE(data.length, 22);
  local.writeUInt16LE(nameBytes.length, 26);
  local.writeUInt16LE(0, 28);

  const central = Buffer.alloc(46);
  central.writeUInt32LE(0x02014b50, 0);
  central.writeUInt16LE(20, 4);
  central.writeUInt16LE(20, 6);
  central.writeUInt16LE(0, 8);
  central.writeUInt16LE(0, 10);
  central.writeUInt16LE(0, 12);
  central.writeUInt16LE(0x21, 14);
  central.writeUInt32LE(sum, 16);
  central.writeUInt32LE(data.length, 20);
  central.writeUInt32LE(data.length, 24);
  central.writeUInt16LE(nameBytes.length, 28);
  central.writeUInt32LE(0, 42);

  const centralOffset = local.length + nameBytes.length + data.length;
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50, 0);
  end.writeUInt16LE(1, 8);
  end.writeUInt16LE(1, 10);
  end.writeUInt32LE(central.length + nameBytes.length, 12);
  end.writeUInt32LE(centralOffset, 16);

  return Buffer.concat([local, nameBytes, data, central, nameBytes, end]);
}

const SKILL_MD = `---
name: stack-smoke-seed
description: A seeded skill so the browser pass has a page with content on it.
license: MIT
---

# Task

Summarise the text the user provides, in the format they ask for.

## Steps

1. Read the input.
2. Ask which output format is wanted when it is not stated.
3. Answer in that format.
`;

export async function seedSkill(request, base) {
  const res = await request.post(base + "/skills/import/upload", {
    headers: { "content-type": "application/zip" },
    data: zipOneFile("SKILL.md", SKILL_MD),
  });
  if (res.status() !== 201) {
    throw new Error(
      `seed upload answered ${res.status()}: ${(await res.text()).slice(0, 300)}`,
    );
  }
  const body = await res.json();
  if (!body.skill_id || !body.version_id) {
    throw new Error(
      `seed upload returned no ids: ${JSON.stringify(body).slice(0, 200)}`,
    );
  }

  const tc = await request.post(base + "/test-cases", {
    data: {
      skill_id: body.skill_id,
      name: "煙霧測試題",
      user_prompt: "把下面這段文字整理成三個重點。",
    },
  });
  if (tc.status() !== 201) {
    throw new Error(
      `seed test case answered ${tc.status()}: ${(await tc.text()).slice(0, 300)}`,
    );
  }
  const tcBody = await tc.json();
  if (!tcBody.test_case_id) {
    throw new Error(
      `seed test case returned no id: ${JSON.stringify(tcBody).slice(0, 200)}`,
    );
  }

  return {
    SKILL: body.skill_id,
    VERSION: body.version_id,
    TEST_CASE: tcBody.test_case_id,
  };
}
