# mcgsUdiskCopy
Linux环境下， 批量复制文件夹到U盘小工具

# 说明
- 默认复制到U盘根目录下tpcbackup目录
- U盘查找通过/proc/mounts文件解析vfat文件系统类型，所以要求U为fat格式，且已经被挂载
- 支持各U盘文件目录的并发拷贝以及进度条显示

# 使用方法 
$ mcgsUdiskCopy -i <src>
