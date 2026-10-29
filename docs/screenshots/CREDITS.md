# 界面截图与风景照片

本目录为原生 Mac 相框和网页版的实际运行截图，使用独立的风景演示数据。未使用家庭照片、个人图库或私人服务配置；演示实例与日常相框分开运行。

截图中的照片来自仓库已有的 `themes/nature-calm/wallpapers/`，原始出处及授权完整保留在[风景主题的素材说明](../../themes/nature-calm/CREDITS.md)。

本次使用了以下六张素材，大图与底部缩略图均包含在内：

| 素材 | 作者 | 授权 |
| --- | --- | --- |
| [老挝喀斯特山景与晨雾](https://commons.wikimedia.org/wiki/File:Karst_mountains_colorful_clouds_and_mist_at_sunrise_from_the_top_of_Mount_Nam_Xay_Vang_Vieng_Laos.jpg) | Basile Morin | CC BY-SA 4.0 |
| [图梅尔湖倒影](https://commons.wikimedia.org/wiki/File:Foggy_Reflections_-_Loch_Tummel.jpg) | Eric Kilby | CC BY-SA 4.0 |
| [新西兰 Mystery 湖](https://commons.wikimedia.org/wiki/File:Lake_Mystery,_Canterbury,_New_Zealand_05.jpg) | Michal Klajban | CC BY-SA 4.0 |
| [林间蓝铃花](https://commons.wikimedia.org/wiki/File:Pryor%27s_Wood_Bluebells_2017-04-26-4.jpg) | Colin | CC BY-SA 4.0 |
| [Lierneux 苔藓森林](https://commons.wikimedia.org/wiki/File:Mossy_forest_in_Lierneux_(DSC01260).jpg) | Trougnouf | CC BY 4.0 |
| [泰国 Cheow Lan 湖](https://commons.wikimedia.org/wiki/File:Cheow_Lan_Lake,_Rainforest,_Khao_Sok,_Thailand.jpg) | Vyacheslav Argenberg | CC BY 4.0 |

主题中的照片已做过裁切、缩放和局部压暗。本次由应用继续缩放显示、生成缩略图和柔和背景，并叠加实际界面控件；没有进行 AI 扩图。

本目录两张截图按 [CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/) 提供；这不改变程序代码原有的许可。原始素材同时保留各自的 [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/) 或 CC BY-SA 4.0 许可。

## 本地扩图演示

`outpaint-source.jpg`、`outpaint-before.jpg` 与 `outpaint-after.jpg` 使用上表的老挝山景（Basile Morin，CC BY-SA 4.0）。在仓库已有风景素材的基础上，居中裁成 9:16 竖幅，再通过当前的本地 FLUX Fill 工作流生成两侧背景，最后使用渐进柔化，并恢复原照片中央区域。

- `outpaint-source.jpg`：竖幅输入。
- `outpaint-before.jpg`：将竖幅输入居中放在横向画布上，两侧留空，用于对比；不是电视实际播放时的黑边截图。
- `outpaint-after.jpg`：实际生成并完成柔化的 1920 × 1080 成品。

这组图片按 CC BY-SA 4.0 提供。新增景物由 AI 生成，不是原拍摄地点的真实记录。本次为独立演示任务，没有修改日常筛选条件或把演示图加入家庭照片批次。
