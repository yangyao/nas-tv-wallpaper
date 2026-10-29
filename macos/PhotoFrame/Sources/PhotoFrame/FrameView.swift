import SwiftUI

struct FrameView: View {
    @ObservedObject var model: FrameModel
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    var body: some View {
        GeometryReader { bounds in
            ZStack(alignment: .bottom) {
                Color(red: 0.08, green: 0.09, blue: 0.1)
                if let image = model.picture {
                    ZStack {
                        Image(nsImage: image).resizable().scaledToFill()
                            .frame(width: bounds.size.width, height: bounds.size.height)
                            .clipped().blur(radius: 45).scaleEffect(1.12)
                            .overlay(Color.black.opacity(0.18))
                        Image(nsImage: image).resizable().scaledToFit()
                            .frame(width: bounds.size.width, height: bounds.size.height)
                    }
                    .id(model.pictureID)
                    .transition(.opacity)
                    .allowsHitTesting(false)
                }
                if model.controls {
                    controls.frame(width: min(880, max(240, bounds.size.width - 64)))
                        .padding(.bottom, 24)
                        .transition(.opacity.combined(with: .move(edge: .bottom)))
                }
            }
            .frame(width: bounds.size.width, height: bounds.size.height)
            .clipped()
            .animation(reduceMotion ? nil : .easeInOut(duration: 0.65), value: model.pictureID)
            .animation(reduceMotion ? nil : .easeOut(duration: 0.22), value: model.controls)
        }
        .ignoresSafeArea()
        .sheet(isPresented: $model.settings) { configuration }
    }
    private var controls: some View {
        VStack(spacing: 10) {
            ScrollViewReader { reader in
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 12) {
                        ForEach(Array(model.timeline.ids.enumerated()), id: \.element) { index, id in
                            Button { model.select(index); model.reveal() } label: {
                                Group {
                                    if let thumb = model.thumbnails[id] {
                                        Image(nsImage: thumb).resizable().scaledToFill()
                                    } else { Color.gray }
                                }
                                .frame(width: 92, height: 68).clipped()
                                .clipShape(RoundedRectangle(cornerRadius: 10))
                                .overlay(RoundedRectangle(cornerRadius: 12)
                                    .stroke(.white.opacity(id == model.timeline.current ? 0.98 : 0), lineWidth: 3)
                                    .padding(-3))
                                .scaleEffect(id == model.timeline.current ? 1.04 : 1)
                            }
                            .buttonStyle(.plain).id(id)
                            .accessibilityLabel("照片 \(index + 1)")
                        }
                    }.padding(18)
                }
                .frame(height: 108)
                .background(GlassPanel(radius: 26))
                .onAppear { reader.scrollTo(model.timeline.current, anchor: .center) }
                .onChange(of: model.timeline.current) { id in
                    withAnimation(reduceMotion ? nil : .spring(response: 0.38, dampingFraction: 0.86)) {
                        reader.scrollTo(id, anchor: .center)
                    }
                }
            }
            HStack(spacing: 4) {
                action("chevron.left", "上一张") { model.move(-1); model.reveal() }
                action(model.playing ? "pause.fill" : "play.fill", model.playing ? "暂停" : "播放") { model.togglePlayback() }
                action("chevron.right", "下一张") { model.move(1); model.reveal() }
                action("arrow.up.left.and.arrow.down.right", "全屏") { model.window?.toggleFullScreen(nil); model.reveal() }
                action("gearshape", "设置") { model.settings = true }
            }
            .padding(5).background(GlassPanel(radius: 23))
        }
    }
    private func action(_ icon: String, _ label: String, _ work: @escaping () -> Void) -> some View {
        Button(action: work) { Image(systemName: icon).font(.system(size: 17, weight: .medium)).frame(width: 40, height: 34) }
            .buttonStyle(.plain).foregroundStyle(Color(red: 0.15, green: 0.19, blue: 0.22))
            .accessibilityLabel(label).help(label)
    }
    private var configuration: some View {
        VStack(alignment: .leading, spacing: 18) {
            Text("照片相框").font(.title2.weight(.semibold))
            Text("NAS 服务地址").font(.headline)
            TextField("https://photos.example.com:8443", text: $model.address)
                .textFieldStyle(.roundedBorder).onSubmit { model.configure() }
            Text("仅连接你信任的家庭网络服务。照片在本机有限缓存，不上传到云端。")
                .font(.callout).foregroundStyle(.secondary)
            HStack {
                Spacer()
                Button("取消") { model.settings = false }.keyboardShortcut(.cancelAction)
                Button("保存") { model.configure() }.keyboardShortcut(.defaultAction)
                    .disabled(ServerAddress.parse(model.address) == nil)
            }
        }.padding(28).frame(width: 430)
    }
}

private struct GlassPanel: View {
    let radius: CGFloat
    var body: some View {
        RoundedRectangle(cornerRadius: radius)
            .fill(.regularMaterial)
            .overlay(RoundedRectangle(cornerRadius: radius).fill(Color(red: 0.91, green: 0.94, blue: 0.96).opacity(0.72)))
            .overlay(RoundedRectangle(cornerRadius: radius).stroke(.white.opacity(0.55), lineWidth: 1))
            .environment(\.colorScheme, .light)
    }
}
