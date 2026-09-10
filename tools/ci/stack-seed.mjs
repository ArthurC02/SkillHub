// Put one real Skill into the stack, so the browser pass has pages with content
// on them. The rest of 04 丙-221.
//
// Why a zip is built here rather than committed. `POST /skills/import/upload`
// takes an `application/zip` body, and the archive is the product's own trust
// boundary: a committed binary fixture is a thing nobody reads again, and the
// one property this seed needs (a SKILL.md at the top level with name and
// description in its frontmatter) is exactly what a reader has to be able to
// see. So the archive is written here, in fifty lines, stored not deflated —
// the platform accepts Store, and a deflate implementation would be a
// dependency to prove nothing.
//
// Deliberately minimal content. This is not a corpus: the routes under test
// need a skill that exists, has a version and passes static validation. What a
// realistic package looks like is tools/qa/skillpkg-corpus's job.

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

/** A store-only zip of one file. Enough for the importer, and readable. */
export function zipOneFile(name, text) {
  const nameBytes = Buffer.from(name, "utf8");
  const data = Buffer.from(text, "utf8");
  const sum = crc32(data);

  const local = Buffer.alloc(30);
  local.writeUInt32LE(0x04034b50, 0); // local file header
  local.writeUInt16LE(20, 4); // version needed
  local.writeUInt16LE(0, 6); // flags
  local.writeUInt16LE(0, 8); // method 0 = stored
  local.writeUInt16LE(0, 10); // time
  local.writeUInt16LE(0x21, 12); // date (1996-01-01; zip has no "unset")
  local.writeUInt32LE(sum, 14);
  local.writeUInt32LE(data.length, 18);
  local.writeUInt32LE(data.length, 22);
  local.writeUInt16LE(nameBytes.length, 26);
  local.writeUInt16LE(0, 28); // extra length

  const central = Buffer.alloc(46);
  central.writeUInt32LE(0x02014b50, 0); // central directory header
  central.writeUInt16LE(20, 4); // version made by
  central.writeUInt16LE(20, 6); // version needed
  central.writeUInt16LE(0, 8);
  central.writeUInt16LE(0, 10);
  central.writeUInt16LE(0, 12);
  central.writeUInt16LE(0x21, 14);
  central.writeUInt32LE(sum, 16);
  central.writeUInt32LE(data.length, 20);
  central.writeUInt32LE(data.length, 24);
  central.writeUInt16LE(nameBytes.length, 28);
  central.writeUInt32LE(0, 42); // offset of local header

  const centralOffset = local.length + nameBytes.length + data.length;
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50, 0); // end of central directory
  end.writeUInt16LE(1, 8); // entries on this disk
  end.writeUInt16LE(1, 10); // entries total
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

/**
 * Puts the seed content in with the caller's session and returns the ids the
 * routes need. Throws with the server's own words on any other status: a seed
 * that quietly failed would turn every "not found" page green for the wrong
 * reason, which is worse than not seeding at all.
 *
 * A Test Case is seeded here and a Run is not, and the line between them is not
 * effort. POST /test-cases takes a skill id, a name and a prompt and calls
 * nothing; a Run has to reach a model (05 R-35: no model route, no dispatch),
 * so it costs money and waits on the cadence 05 R-72 rules. 04 丙-227 said both
 * were blocked on the same thing because they were noticed together.
 */
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
