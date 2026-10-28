// Local-only Apple Vision detector. Coordinates use the top-left image origin.
import AppKit
import Vision
import Foundation

guard CommandLine.arguments.count == 2,
      let image = NSImage(contentsOfFile: CommandLine.arguments[1]),
      let cg = image.cgImage(forProposedRect: nil, context: nil, hints: nil) else {
    fputs("Cannot read image\n", stderr); exit(1)
}
do {
    let faces = VNDetectFaceRectanglesRequest()
    let people = VNDetectHumanRectanglesRequest()
    people.upperBodyOnly = true
    try VNImageRequestHandler(cgImage: cg, options: [:]).perform([faces, people])
    func record(_ item: VNDetectedObjectObservation, _ kind: String) -> [String: Any] {
        let b = item.boundingBox
        return ["kind": kind, "confidence": Double(item.confidence),
                "box": [Double(b.minX), Double(1-b.maxY), Double(b.width), Double(b.height)]]
    }
    var boxes = (faces.results ?? []).map { record($0, "face") }
    boxes += (people.results ?? []).map { record($0, "person") }
    print(String(data: try JSONSerialization.data(withJSONObject: boxes), encoding: .utf8)!)
} catch {
    fputs("Local people detection failed\n", stderr); exit(1)
}
