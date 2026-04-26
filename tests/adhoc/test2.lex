struct Rect {
    w, h
    fn area() { return self.w * self.h }
    fn scale(f) { self.w = self.w * f   self.h = self.h * f }
}

r = Rect { w: 4, h: 5 }
println(r.area()) // 20