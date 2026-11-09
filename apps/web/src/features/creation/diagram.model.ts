import { byteLimit } from "../../shared/format";

const DIAGRAM_TYPES = ["image/png", "image/jpeg", "image/webp"];

export function diagramProblem(file: File, maxBytes: number): string | undefined {
  if (!DIAGRAM_TYPES.includes(file.type)) return "流程圖只收 PNG、JPEG 或 WebP。";
  if (file.size === 0 || file.size > maxBytes)
    return `請選擇最多 ${byteLimit(maxBytes)} 以內的流程圖。`;
  return undefined;
}

export function readDiagram(
  file: File,
  maxBytes: number,
): Promise<{ media_type: string; data: string }> {
  const problem = diagramProblem(file, maxBytes);
  if (problem) return Promise.reject(new Error(problem));
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(new Error("流程圖無法讀取，請重新選擇。"));
    reader.onload = () =>
      resolve({ media_type: file.type, data: String(reader.result).split(",")[1] });
    reader.readAsDataURL(file);
  });
}
