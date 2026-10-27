const BLOCK: usize = 512;

pub struct Member {
    pub name: String,
    pub data: Vec<u8>,
}

pub fn regular_files(archive: &[u8]) -> Result<Vec<Member>, String> {
    let mut members = Vec::new();
    let mut at = 0;
    let mut long_name: Option<String> = None;
    while at + BLOCK <= archive.len() {
        let header = &archive[at..at + BLOCK];
        if header.iter().all(|byte| *byte == 0) {
            break;
        }
        let size = octal(&header[124..136])?;
        let body_start = at + BLOCK;
        let body = archive
            .get(body_start..body_start + size)
            .ok_or("unexpected end of data")?;
        at = body_start + size.div_ceil(BLOCK) * BLOCK;
        match header[156] {
            b'x' => long_name = pax_path(body),
            b'g' => {}
            b'0' | 0 | b'7' => members.push(Member {
                name: long_name.take().unwrap_or_else(|| ustar_name(header)),
                data: body.to_vec(),
            }),
            _ => long_name = None,
        }
    }
    Ok(members)
}

fn octal(field: &[u8]) -> Result<usize, String> {
    let text: String = field
        .iter()
        .take_while(|byte| **byte != 0)
        .map(|byte| char::from(*byte))
        .collect();
    let digits = text.trim();
    if digits.is_empty() {
        return Ok(0);
    }
    usize::from_str_radix(digits, 8).map_err(|_| "invalid header".to_string())
}

fn text_field(field: &[u8]) -> String {
    let end = field
        .iter()
        .position(|byte| *byte == 0)
        .unwrap_or(field.len());
    String::from_utf8_lossy(&field[..end]).into_owned()
}

fn ustar_name(header: &[u8]) -> String {
    let name = text_field(&header[0..100]);
    let prefix = if &header[257..262] == b"ustar" {
        text_field(&header[345..500])
    } else {
        String::new()
    };
    if prefix.is_empty() {
        name
    } else {
        format!("{prefix}/{name}")
    }
}

fn pax_path(body: &[u8]) -> Option<String> {
    let mut at = 0;
    let mut path = None;
    while at < body.len() {
        let space = body[at..].iter().position(|byte| *byte == b' ')? + at;
        let length: usize = std::str::from_utf8(&body[at..space]).ok()?.parse().ok()?;
        let record = body.get(space + 1..at + length)?;
        let record = record.strip_suffix(b"\n").unwrap_or(record);
        if let Some(value) = record.strip_prefix(b"path=") {
            path = Some(String::from_utf8_lossy(value).into_owned());
        }
        at += length;
    }
    path
}
