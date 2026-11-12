# gomobile runtime and generated bindings are called through JNI.
-keep class go.** { *; }
-keep class io.github.sqmydor.surfmi.core.libcore.** { *; }
# Kotlin implementations of Go interfaces are invoked from native code.
-keep class * implements io.github.sqmydor.surfmi.core.libcore.Platform { *; }
-keepclassmembers class * implements go.Seq$Proxy { *; }
-dontwarn go.**
