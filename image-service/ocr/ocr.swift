// ocr <image>: prints each line of text Apple Vision reads in the image.
//
// The image service reads a generated picture's lettering back with this
// (app/textcheck.py) and redraws one whose labels came out misspelt or
// repeated. run.sh compiles it; nothing is downloaded, Vision ships with macOS.
import Foundation
import Vision

let url = URL(fileURLWithPath: CommandLine.arguments[1])
let request = VNRecognizeTextRequest()
request.recognitionLevel = .accurate
// Off: correction would "fix" a misspelt label into the word it should have
// been, which is exactly the mistake this is here to catch.
request.usesLanguageCorrection = false
try VNImageRequestHandler(url: url).perform([request])
for observation in request.results ?? [] {
    if let line = observation.topCandidates(1).first { print(line.string) }
}
