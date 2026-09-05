param([string]$Corpus, [string]$OutDir)
Add-Type -AssemblyName System.Drawing
New-Item -ItemType Directory -Force $OutDir | Out-Null
$json = Get-Content $Corpus -Raw -Encoding UTF8 | ConvertFrom-Json
$fonts = @("Microsoft JhengHei", "Segoe Print", "Arial", "Microsoft JhengHei UI")
$i = 0
foreach ($d in $json.diagram) {
  $n = $d.nodes.Count
  $boxW = 360; $boxH = 74; $gap = 56; $left = 170; $top = 50
  $W = 810; $H = $top * 2 + $n * $boxH + ($n - 1) * $gap
  $bmp = New-Object System.Drawing.Bitmap $W, $H
  $g = [System.Drawing.Graphics]::FromImage($bmp)
  $g.SmoothingMode = "AntiAlias"; $g.TextRenderingHint = "AntiAliasGridFit"
  $bg = if ($i % 3 -eq 2) { [System.Drawing.Color]::FromArgb(255, 252, 248, 230) } else { [System.Drawing.Color]::White }
  $g.Clear($bg)
  $font = New-Object System.Drawing.Font($fonts[$i % $fonts.Count], 15)
  $small = New-Object System.Drawing.Font($fonts[$i % $fonts.Count], 12)
  $pen = New-Object System.Drawing.Pen([System.Drawing.Color]::Black, 2)
  $pen.CustomEndCap = New-Object System.Drawing.Drawing2D.AdjustableArrowCap 6, 8
  $brush = [System.Drawing.Brushes]::Black
  $fmt = New-Object System.Drawing.StringFormat
  $fmt.Alignment = "Center"; $fmt.LineAlignment = "Center"
  $decIdx = @{}
  foreach ($dc in $d.decisions) { $decIdx[[int]$dc.index] = $dc }
  $next = @{}
  if ($d.PSObject.Properties["next"]) { foreach ($p in $d.next.PSObject.Properties) { $next[[int]$p.Name] = [int]$p.Value } }
  $rects = @()
  for ($k = 0; $k -lt $n; $k++) {
    $y = $top + $k * ($boxH + $gap)
    $r = New-Object System.Drawing.RectangleF $left, $y, $boxW, $boxH
    $rects += $r
    if ($decIdx.ContainsKey($k)) {
      $pts = @((New-Object System.Drawing.PointF ($left + $boxW / 2), $y), (New-Object System.Drawing.PointF ($left + $boxW), ($y + $boxH / 2)), (New-Object System.Drawing.PointF ($left + $boxW / 2), ($y + $boxH)), (New-Object System.Drawing.PointF $left, ($y + $boxH / 2)))
      $g.FillPolygon([System.Drawing.Brushes]::White, $pts); $g.DrawPolygon($pen, $pts)
    } elseif ($k -eq 0 -or $k -eq $n - 1) {
      $path = New-Object System.Drawing.Drawing2D.GraphicsPath
      $path.AddArc($r.X, $r.Y, $boxH, $boxH, 90, 180); $path.AddArc($r.X + $boxW - $boxH, $r.Y, $boxH, $boxH, 270, 180); $path.CloseFigure()
      $g.FillPath([System.Drawing.Brushes]::White, $path); $g.DrawPath($pen, $path)
    } else {
      $g.FillRectangle([System.Drawing.Brushes]::White, $r); $g.DrawRectangle($pen, $r.X, $r.Y, $r.Width, $r.Height)
    }
    $g.DrawString($d.nodes[$k].label, $font, $brush, $r, $fmt)
  }
  # edges
  $plainPen = New-Object System.Drawing.Pen([System.Drawing.Color]::Black, 2)
  $plainPen.CustomEndCap = New-Object System.Drawing.Drawing2D.AdjustableArrowCap 6, 8
  $rightLane = 0; $leftLane = 0
  for ($k = 0; $k -lt $n; $k++) {
    $r = $rects[$k]
    $cx = $r.X + $r.Width / 2
    if ($decIdx.ContainsKey($k)) {
      $dc = $decIdx[$k]
      foreach ($pair in @(@($dc.yes, [int]$dc.yes_to, "yes"), @($dc.no, [int]$dc.no_to, "no"))) {
        $lbl = $pair[0]; $to = $pair[1]; $side = $pair[2]
        if ($to -lt 0) { continue }
        $t = $rects[$to]
        if ($to -eq $k + 1) {
          $g.DrawLine($plainPen, $cx, $r.Bottom, $cx, $t.Top)
          $g.DrawString($lbl, $small, $brush, $cx + 6, $r.Bottom + 8)
        } else {
          $rightLane++
          $x = $r.Right + 40 + $rightLane * 30
          if ($side -eq "no") { $startX = $r.Right; $startY = $r.Y + $r.Height / 2 } else { $startX = $r.X; $startY = $r.Y + $r.Height / 2; $leftLane++; $x = $r.X - 40 - $leftLane * 30 }
          $g.DrawLine($pen.Clone(), $startX, $startY, $x, $startY) | Out-Null
          $g.DrawLine((New-Object System.Drawing.Pen([System.Drawing.Color]::Black, 2)), $startX, $startY, $x, $startY)
          $g.DrawLine((New-Object System.Drawing.Pen([System.Drawing.Color]::Black, 2)), $x, $startY, $x, $t.Y + $t.Height / 2)
          $endX = if ($side -eq "no") { $t.Right } else { $t.X }
          $g.DrawLine($plainPen, $x, $t.Y + $t.Height / 2, $endX, $t.Y + $t.Height / 2)
          $g.DrawString($lbl, $small, $brush, ($startX + $x) / 2 - 10, $startY - 22)
        }
      }
    } elseif ($k -lt $n - 1) {
      $to = $k + 1
      if ($next.ContainsKey($k)) { $to = $next[$k] }
      if ($to -lt 0) { continue }
      $t = $rects[$to]
      if ($to -eq $k + 1) {
        $g.DrawLine($plainPen, $cx, $r.Bottom, $cx, $t.Top)
      } else {
        $leftLane++
        $x = $r.X - 40 - $leftLane * 30
        $g.DrawLine((New-Object System.Drawing.Pen([System.Drawing.Color]::Black, 2)), $r.X, $r.Y + $r.Height / 2, $x, $r.Y + $r.Height / 2)
        $g.DrawLine((New-Object System.Drawing.Pen([System.Drawing.Color]::Black, 2)), $x, $r.Y + $r.Height / 2, $x, $t.Y + $t.Height / 2)
        $g.DrawLine($plainPen, $x, $t.Y + $t.Height / 2, $t.X, $t.Y + $t.Height / 2)
      }
    }
  }
  $ext = $d.media
  $file = Join-Path $OutDir "$($d.id).$ext"
  if ($ext -eq "jpg") {
    $codec = [System.Drawing.Imaging.ImageCodecInfo]::GetImageEncoders() | Where-Object { $_.MimeType -eq "image/jpeg" }
    $ep = New-Object System.Drawing.Imaging.EncoderParameters 1
    $ep.Param[0] = New-Object System.Drawing.Imaging.EncoderParameter([System.Drawing.Imaging.Encoder]::Quality, [long]85)
    $bmp.Save($file, $codec, $ep)
  } else {
    $bmp.Save($file, [System.Drawing.Imaging.ImageFormat]::Png)
  }
  $g.Dispose(); $bmp.Dispose()
  Write-Output "$($d.id) $ext ${W}x${H} $((Get-Item $file).Length) bytes"
  $i++
}
