// Local Apple Vision detection; no external inference service or model download.
import AppKit
import Vision
import Foundation

guard CommandLine.arguments.count == 2,
      let image = NSImage(contentsOfFile: CommandLine.arguments[1]),
      let cg = image.cgImage(forProposedRect: nil, context: nil, hints: nil) else {
    fputs("Cannot read source image\n", stderr); exit(1)
}
do {
    let request = VNDetectFaceRectanglesRequest()
    try VNImageRequestHandler(cgImage: cg, options: [:]).perform([request])
    let boxes = (request.results ?? []).map { face -> [Double] in
        let b = face.boundingBox
        return [Double(b.minX), Double(1 - b.maxY), Double(b.width), Double(b.height)]
    }
    let data = try JSONSerialization.data(withJSONObject: boxes)
    print(String(data: data, encoding: .utf8)!)
} catch {
    fputs("Local face detection failed\n", stderr); exit(1)
}
