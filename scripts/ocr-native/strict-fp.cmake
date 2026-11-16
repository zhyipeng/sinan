# Append after upstream targets are declared, without editing upstream sources.
# v5.0.0a6 enables fast-math; local AppleClang 21 runs lost lines intermittently.
if(MSVC)
  cmake_language(DEFER CALL target_compile_options ppocr_objects PRIVATE /fp:precise)
else()
  cmake_language(DEFER CALL target_compile_options ppocr_objects PRIVATE -fno-fast-math)
endif()
