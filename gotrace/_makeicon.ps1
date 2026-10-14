Add-Type -AssemblyName System.Drawing
$size = 256
$bmp = New-Object System.Drawing.Bitmap($size, $size)
$g = [System.Drawing.Graphics]::FromImage($bmp)
$g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias

$bgBrush = New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb(255, 0x17, 0x5C, 0xD3))
$g.FillEllipse($bgBrush, 6, 6, 244, 244)
$c1 = $bmp.GetPixel(20, 128)
Write-Output ("after bg (20,128): A={0} R={1} G={2} B={3}" -f $c1.A, $c1.R, $c1.G, $c1.B)

$routePen = New-Object System.Drawing.Pen([System.Drawing.Color]::White, 18)
$routePen.EndCap = [System.Drawing.Drawing2D.LineCap]::Round
$routePen.StartCap = [System.Drawing.Drawing2D.LineCap]::Round
$routePen.LineJoin = [System.Drawing.Drawing2D.LineJoin]::Round
$pts = @(
    (New-Object System.Drawing.Point(52, 204)),
    (New-Object System.Drawing.Point(100, 150)),
    (New-Object System.Drawing.Point(152, 172)),
    (New-Object System.Drawing.Point(198, 96))
)
$g.DrawLines($routePen, $pts)

$nodeBrush = New-Object System.Drawing.SolidBrush([System.Drawing.Color]::White)
foreach ($pt in @($pts[0], $pts[1], $pts[2])) {
    $g.FillEllipse($nodeBrush, $pt.X-16, $pt.Y-16, 32, 32)
}

$greenBrush = New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb(255, 0x22, 0xC5, 0x5E))
$g.FillEllipse($greenBrush, 198-30, 96-30, 60, 60)
$ringPen = New-Object System.Drawing.Pen([System.Drawing.Color]::White, 8)
$g.DrawEllipse($ringPen, 198-30, 96-30, 60, 60)

$c2 = $bmp.GetPixel(20, 128)
Write-Output ("before save (20,128): A={0} R={1} G={2} B={3}" -f $c2.A, $c2.R, $c2.G, $c2.B)
$g.Dispose()
$bmp.Save('E:\tools\nexttrace\gotrace\winres\icon.png', [System.Drawing.Imaging.ImageFormat]::Png)
$bmp.Dispose()
Write-Output "saved"
