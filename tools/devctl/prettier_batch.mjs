import { pathToFileURL } from "node:url";

const prettier = await import(pathToFileURL(process.argv[2]).href);
const chunks = [];
for await (const chunk of process.stdin) chunks.push(chunk);
const input = Buffer.concat(chunks).toString("utf8");

const ignorePath = [".gitignore", ".prettierignore"];
const results = await Promise.all(
  JSON.parse(input).map(async ({ file, source }) => {
    try {
      const info = await prettier.getFileInfo(file, { ignorePath, resolveConfig: true });
      if (info.ignored) return { formatted: source };
      if (!info.inferredParser) return { unformattable: true };
      const options = await prettier.resolveConfig(file, { editorconfig: true });
      return { formatted: await prettier.format(source, { ...options, filepath: file }) };
    } catch (error) {
      return { error: String(error?.message ?? error).split("\n")[0] };
    }
  }),
);
process.stdout.write(JSON.stringify(results));
